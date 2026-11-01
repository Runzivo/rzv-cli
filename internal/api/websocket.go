package api

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // RFC 6455 defines the handshake with SHA-1; it is not used for security here.
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var zeroTime time.Time

// A WebSocket client (RFC 6455) small enough to read in one sitting: one
// connection, binary and text messages, ping/pong and close. It exists because
// it predates the SSH integration; it is not a general library.

const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA

	websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	// maxMessageBytes bounds one received message; terminal output arrives in
	// small frames, so this is a safety limit, not a working size.
	maxMessageBytes = 4 << 20
)

// CloseError is the server's close frame. Code 1000 is a normal end and Reason
// then says why the terminal ended (for example sandbox_stopped).
type CloseError struct {
	Code   int
	Reason string
}

func (e *CloseError) Error() string {
	return fmt.Sprintf("connection closed (%d %s)", e.Code, e.Reason)
}

// Message types returned by ReadMessage.
const (
	MessageText   = opText
	MessageBinary = opBinary
)

// WebSocket is one client connection. Reads come from a single goroutine;
// writes may come from several.
type WebSocket struct {
	conn    net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
	closed  bool
}

// dialWebSocket opens target (ws or wss) with header added to the handshake.
// A refusal before the upgrade comes back as *Error with the server's code.
func dialWebSocket(ctx context.Context, target string, header http.Header) (*WebSocket, error) {
	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") || parsed.Host == "" {
		return nil, &Error{Status: 0, Code: "request_failed"}
	}
	address := parsed.Host
	if parsed.Port() == "" {
		if parsed.Scheme == "wss" {
			address = net.JoinHostPort(parsed.Hostname(), "443")
		} else {
			address = net.JoinHostPort(parsed.Hostname(), "80")
		}
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, &Error{Status: 0, Code: "customer_api_unavailable"}
	}
	if parsed.Scheme == "wss" {
		secured := tls.Client(conn, &tls.Config{ServerName: parsed.Hostname(), MinVersion: tls.VersionTLS12})
		if err := secured.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, &Error{Status: 0, Code: "customer_api_unavailable"}
		}
		conn = secured
	}
	socket, err := handshake(ctx, conn, parsed, header)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return socket, nil
}

func handshake(ctx context.Context, conn net.Conn, target *url.URL, header http.Header) (*WebSocket, error) {
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	rawKey := make([]byte, 16)
	if _, err := rand.Read(rawKey); err != nil {
		return nil, &Error{Status: 0, Code: "request_failed"}
	}
	key := base64.StdEncoding.EncodeToString(rawKey)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, &Error{Status: 0, Code: "request_failed"}
	}
	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Sec-WebSocket-Key", key)
	request.Header.Set("Sec-WebSocket-Version", "13")
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if err := request.Write(conn); err != nil {
		return nil, &Error{Status: 0, Code: "customer_api_unavailable"}
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, &Error{Status: 0, Code: "customer_api_unavailable"}
	}
	defer func() {
		if response.StatusCode != http.StatusSwitchingProtocols {
			_ = response.Body.Close()
		}
	}()
	if response.StatusCode != http.StatusSwitchingProtocols {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return nil, decodeError(response.StatusCode, data)
	}
	sum := sha1.Sum([]byte(key + websocketGUID)) //nolint:gosec
	if !strings.EqualFold(response.Header.Get("Upgrade"), "websocket") ||
		response.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		return nil, &Error{Status: response.StatusCode, Code: "malformed_response"}
	}
	_ = conn.SetDeadline(zeroTime)
	return &WebSocket{conn: conn, reader: reader}, nil
}

// WriteBinary sends terminal bytes.
func (w *WebSocket) WriteBinary(data []byte) error { return w.writeFrame(opBinary, data) }

// WriteText sends one text message (the window size).
func (w *WebSocket) WriteText(data []byte) error { return w.writeFrame(opText, data) }

// Close sends a normal close and closes the connection.
func (w *WebSocket) Close() error {
	_ = w.conn.SetWriteDeadline(time.Now().Add(time.Second))
	w.writeMu.Lock()
	already := w.closed
	w.writeMu.Unlock()
	if !already {
		_ = w.writeFrame(opClose, []byte{0x03, 0xe8}) // 1000
	}
	return w.conn.Close()
}

// writeFrame sends one masked, unfragmented frame, as a client must.
func (w *WebSocket) writeFrame(opcode byte, payload []byte) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	if w.closed && opcode != opClose {
		return io.ErrClosedPipe
	}
	if opcode == opClose {
		w.closed = true
	}
	header := []byte{0x80 | opcode}
	switch n := len(payload); {
	case n < 126:
		header = append(header, 0x80|byte(n))
	case n <= 0xFFFF:
		header = append(header, 0x80|126, byte(n>>8), byte(n))
	default:
		header = append(header, 0x80|127)
		header = binary.BigEndian.AppendUint64(header, uint64(n))
	}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	header = append(header, mask...)
	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}
	if _, err := w.conn.Write(append(header, masked...)); err != nil {
		return err
	}
	return nil
}

// ReadMessage returns the next text or binary message. Pings are answered and
// pongs skipped. A close frame is answered and returned as *CloseError.
func (w *WebSocket) ReadMessage() (messageType int, payload []byte, err error) {
	var assembled []byte
	var assembledType int
	for {
		fin, opcode, data, err := w.readFrame()
		if err != nil {
			return 0, nil, err
		}
		switch opcode {
		case opPing:
			if err := w.writeFrame(opPong, data); err != nil {
				return 0, nil, err
			}
		case opPong:
		case opClose:
			_ = w.conn.SetWriteDeadline(time.Now().Add(time.Second))
			closeErr := &CloseError{Code: 1005}
			if len(data) >= 2 {
				closeErr.Code = int(binary.BigEndian.Uint16(data))
				closeErr.Reason = string(data[2:])
			}
			_ = w.writeFrame(opClose, data[:min(len(data), 2)])
			return 0, nil, closeErr
		case opText, opBinary:
			if assembled != nil {
				return 0, nil, errProtocol
			}
			if fin {
				return int(opcode), data, nil
			}
			assembled, assembledType = append([]byte{}, data...), int(opcode)
		case opContinuation:
			if assembled == nil {
				return 0, nil, errProtocol
			}
			assembled = append(assembled, data...)
			if len(assembled) > maxMessageBytes {
				return 0, nil, errProtocol
			}
			if fin {
				return assembledType, assembled, nil
			}
		default:
			return 0, nil, errProtocol
		}
	}
}

var errProtocol = errors.New("websocket protocol error")

func (w *WebSocket) readFrame() (fin bool, opcode byte, payload []byte, err error) {
	var head [2]byte
	if _, err = io.ReadFull(w.reader, head[:]); err != nil {
		return false, 0, nil, err
	}
	fin, opcode = head[0]&0x80 != 0, head[0]&0x0F
	if head[0]&0x70 != 0 || head[1]&0x80 != 0 { // reserved bits; servers never mask
		return false, 0, nil, errProtocol
	}
	length := uint64(head[1] & 0x7F)
	switch length {
	case 126:
		var extra [2]byte
		if _, err = io.ReadFull(w.reader, extra[:]); err != nil {
			return false, 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(extra[:]))
	case 127:
		var extra [8]byte
		if _, err = io.ReadFull(w.reader, extra[:]); err != nil {
			return false, 0, nil, err
		}
		length = binary.BigEndian.Uint64(extra[:])
	}
	if length > maxMessageBytes || (opcode >= opClose && (length > 125 || !fin)) {
		return false, 0, nil, errProtocol
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(w.reader, payload); err != nil {
		return false, 0, nil, err
	}
	return fin, opcode, payload, nil
}

// Abort interrupts blocked reads and writes without waiting for a close frame.
// A proxy must use this on cancellation, even under backpressure.
func (w *WebSocket) Abort() error { return w.conn.Close() }
