package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/Runzivo/rzv-cli/internal/api"
)

// proxySSH owns the socket and closes pipe ends when their peer ends. Writes
// are synchronous and bounded: neither direction queues unbounded SSH data.
func proxySSH(ctx context.Context, socket *api.WebSocket, input io.Reader, output io.Writer) error {
	defer socket.Abort()
	stop := context.AfterFunc(ctx, func() {
		_ = socket.Abort()
		if c, ok := input.(io.Closer); ok {
			_ = c.Close()
		}
		if c, ok := output.(io.Closer); ok {
			_ = c.Close()
		}
	})
	defer stop()
	defer func() {
		if c, ok := input.(io.Closer); ok {
			_ = c.Close()
		}
	}()
	readDone := make(chan error, 1)
	go func() {
		buffer := make([]byte, 32<<10)
		for {
			n, err := input.Read(buffer)
			if n > 0 {
				if werr := socket.WriteBinary(buffer[:n]); werr != nil {
					readDone <- werr
					_ = socket.Abort()
					return
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = socket.WriteText([]byte(`{"type":"eof"}`))
				}
				readDone <- err
				if err != nil {
					_ = socket.Abort()
				}
				return
			}
		}
	}()
	outputEOF := false
	for {
		kind, data, err := socket.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var closed *api.CloseError
			if errors.As(err, &closed) && closed.Code == 1000 && (closed.Reason == "" || closed.Reason == "shell_exited" || closed.Reason == "socket_closed") {
				return nil
			}
			select {
			case inputErr := <-readDone:
				if inputErr != nil {
					return inputErr
				}
			default:
			}
			return err
		}
		switch kind {
		case api.MessageBinary:
			if outputEOF {
				return errors.New("SSH output after EOF")
			}
			n, err := output.Write(data)
			if err != nil {
				return err
			}
			if n != len(data) {
				return io.ErrShortWrite
			}
		case api.MessageText:
			var frame map[string]json.RawMessage
			if json.Unmarshal(data, &frame) != nil || len(frame) != 1 || string(frame["type"]) != `"eof"` || outputEOF {
				return errors.New("invalid SSH relay control")
			}
			outputEOF = true
			if c, ok := output.(io.Closer); ok {
				if err := c.Close(); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid SSH relay message")
		}
	}
}
