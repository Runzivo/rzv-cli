package api

import (
	"bufio"
	"context"
	"crypto/sha1" //nolint:gosec
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// wsServer accepts one WebSocket and gives the test the raw connection after
// the handshake, so it can send server frames and read the client's.
type wsServer struct {
	URL     string
	headers chan http.Header
	conns   chan *serverConn
}

type serverConn struct {
	net.Conn
	reader *bufio.Reader
}

func newWSServer(t *testing.T, refuse int) *wsServer {
	t.Helper()
	s := &wsServer{headers: make(chan http.Header, 1), conns: make(chan *serverConn, 1)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.headers <- r.Header.Clone()
		if refuse != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(refuse)
			_, _ = w.Write([]byte(`{"error":{"code":"sandbox_wrong_state"}}`))
			return
		}
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + websocketGUID)) //nolint:gosec
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n")
		_ = rw.Flush()
		s.conns <- &serverConn{Conn: conn, reader: rw.Reader}
	}))
	t.Cleanup(server.Close)
	s.URL = "ws" + strings.TrimPrefix(server.URL, "http")
	return s
}

func (c *serverConn) send(t *testing.T, fin bool, opcode byte, payload []byte) {
	t.Helper()
	first := opcode
	if fin {
		first |= 0x80
	}
	frame := []byte{first}
	switch n := len(payload); {
	case n < 126:
		frame = append(frame, byte(n))
	case n <= 0xFFFF:
		frame = append(frame, 126, byte(n>>8), byte(n))
	default:
		frame = append(frame, 127)
		frame = binary.BigEndian.AppendUint64(frame, uint64(n))
	}
	if _, err := c.Write(append(frame, payload...)); err != nil {
		t.Fatal(err)
	}
}

// read returns the next client frame, which must be masked.
func (c *serverConn) read(t *testing.T) (opcode byte, payload []byte) {
	t.Helper()
	var head [2]byte
	if _, err := io.ReadFull(c.reader, head[:]); err != nil {
		t.Fatalf("read frame header: %v", err)
	}
	if head[1]&0x80 == 0 {
		t.Fatal("the client sent an unmasked frame")
	}
	n := int(head[1] & 0x7F)
	if n == 126 {
		var extra [2]byte
		_, _ = io.ReadFull(c.reader, extra[:])
		n = int(binary.BigEndian.Uint16(extra[:]))
	}
	mask := make([]byte, 4)
	_, _ = io.ReadFull(c.reader, mask)
	payload = make([]byte, n)
	_, _ = io.ReadFull(c.reader, payload)
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return head[0] & 0x0F, payload
}

func dial(t *testing.T, s *wsServer) (*WebSocket, *serverConn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	socket, err := dialWebSocket(ctx, s.URL, http.Header{"Authorization": {"Bearer test-key"}})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = socket.conn.Close() })
	return socket, <-s.conns
}

func TestWebSocketHandshakeSendsTheCredentialAndMasksFrames(t *testing.T) {
	server := newWSServer(t, 0)
	socket, conn := dial(t, server)
	header := <-server.headers
	if header.Get("Authorization") != "Bearer test-key" || header.Get("Sec-WebSocket-Version") != "13" || header.Get("Upgrade") != "websocket" {
		t.Fatalf("handshake headers = %v", header)
	}
	if err := socket.WriteBinary([]byte("ls\n")); err != nil {
		t.Fatal(err)
	}
	if opcode, payload := conn.read(t); opcode != opBinary || string(payload) != "ls\n" {
		t.Fatalf("server read %d %q", opcode, payload)
	}
	if err := socket.WriteText(ResizeMessage(120, 40)); err != nil {
		t.Fatal(err)
	}
	if opcode, payload := conn.read(t); opcode != opText || string(payload) != `{"cols":120,"rows":40,"type":"resize"}` {
		t.Fatalf("resize frame = %d %q", opcode, payload)
	}
	// A long message uses the 16 bit length form.
	long := strings.Repeat("x", 70000)
	go func() { _ = socket.WriteBinary([]byte(long)) }()
	var head [2]byte
	_, _ = io.ReadFull(conn.reader, head[:])
	if head[1]&0x7F != 127 {
		t.Fatalf("a 70000 byte frame used length byte %d", head[1]&0x7F)
	}
}

func TestWebSocketReadsMessagesFragmentsPingsAndClose(t *testing.T) {
	server := newWSServer(t, 0)
	socket, conn := dial(t, server)
	conn.send(t, true, opBinary, []byte("output"))
	if kind, data, err := socket.ReadMessage(); err != nil || kind != MessageBinary || string(data) != "output" {
		t.Fatalf("binary = %d %q %v", kind, data, err)
	}
	// A message in three frames with a ping in the middle.
	conn.send(t, false, opBinary, []byte("one "))
	conn.send(t, true, opPing, []byte("hi"))
	conn.send(t, false, opContinuation, []byte("two "))
	conn.send(t, true, opContinuation, []byte("three"))
	if kind, data, err := socket.ReadMessage(); err != nil || kind != MessageBinary || string(data) != "one two three" {
		t.Fatalf("fragmented = %d %q %v", kind, data, err)
	}
	if opcode, payload := conn.read(t); opcode != opPong || string(payload) != "hi" {
		t.Fatalf("the ping was answered with %d %q", opcode, payload)
	}
	conn.send(t, true, opText, []byte("note"))
	if kind, data, err := socket.ReadMessage(); err != nil || kind != MessageText || string(data) != "note" {
		t.Fatalf("text = %d %q %v", kind, data, err)
	}
	closeFrame := append([]byte{0x03, 0xE8}, "sandbox_stopped"...)
	conn.send(t, true, opClose, closeFrame)
	_, _, err := socket.ReadMessage()
	closed, ok := err.(*CloseError)
	if !ok || closed.Code != 1000 || closed.Reason != "sandbox_stopped" {
		t.Fatalf("close = %#v", err)
	}
	if opcode, payload := conn.read(t); opcode != opClose || len(payload) != 2 {
		t.Fatalf("the close was echoed as %d %q", opcode, payload)
	}
}

func TestWebSocketRefusesBrokenOrRefusedConnections(t *testing.T) {
	refused := newWSServer(t, http.StatusConflict)
	_, err := dialWebSocket(context.Background(), refused.URL, http.Header{})
	if apiErr, ok := err.(*Error); !ok || apiErr.Status != 409 || apiErr.Code != "sandbox_wrong_state" {
		t.Fatalf("a refused upgrade = %#v", err)
	}
	server := newWSServer(t, 0)
	socket, conn := dial(t, server)
	conn.send(t, true, opText, nil)
	_, _, _ = socket.ReadMessage()
	// A masked server frame is a protocol error.
	if _, err := conn.Write([]byte{0x82, 0x81, 1, 2, 3, 4, 5}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := socket.ReadMessage(); err == nil {
		t.Fatal("a masked server frame was accepted")
	}
	// So is a control frame that is fragmented or too large, and a stray continuation.
	for name, frame := range map[string][]byte{
		"fragmented ping":    {0x09, 0x00},
		"continuation first": {0x80, 0x01, 'x'},
	} {
		s := newWSServer(t, 0)
		sock, c := dial(t, s)
		if _, err := c.Write(frame); err != nil {
			t.Fatal(err)
		}
		if _, _, err := sock.ReadMessage(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
