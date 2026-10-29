package tunnel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"time"

	"github.com/coder/websocket"

	"github.com/jniltinho/go-guacamole/internal/guac"
)

// Relay limits and timeouts.
const (
	MaxClientElement = 64 << 10
	MaxClientMessage = 1 << 20
	WriteTimeout     = 10 * time.Second
	GuacdIdleTimeout = 30 * time.Second
	batchSize        = 8 << 10
)

// RelayOptions configures one relay run.
type RelayOptions struct {
	// OnActivity is called (at most every 30 s) for user input messages.
	OnActivity func()
}

// SendError writes an "error" instruction and closes the WebSocket with the
// close code/reason contract expected by guacamole-common-js.
func SendError(ws *websocket.Conn, e *guac.Error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if e.Status != guac.StatusSuccess {
		_ = ws.Write(ctx, websocket.MessageText, e.Instruction().Bytes())
	}
	_ = ws.Close(websocket.StatusCode(e.Status.WebSocketCloseCode()), e.Status.Reason())
}

// AsGuacError converts any error to a *guac.Error.
func AsGuacError(err error) *guac.Error {
	var ge *guac.Error
	if errors.As(err, &ge) {
		return ge
	}
	return guac.Errorf(guac.StatusServerError, "internal error")
}

// Relay pumps instructions between the browser and guacd until either side
// closes or ctx is cancelled. It returns the reason the tunnel ended.
func Relay(ctx context.Context, ws *websocket.Conn, g *guac.Conn, uuid string, opt RelayOptions) *guac.Error {
	ws.SetReadLimit(MaxClientMessage)
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	// I/O uses its own context: cancelling a coder/websocket Read/Write context
	// closes the connection abruptly, which would prevent the caller from
	// sending the final "error" instruction and close frame on revocation.
	io0 := context.Background()

	write := func(b []byte) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		wctx, wcancel := context.WithTimeout(io0, WriteTimeout)
		defer wcancel()
		return ws.Write(wctx, websocket.MessageText, b)
	}
	if err := write(guac.New(guac.InternalOpcode, uuid).Bytes()); err != nil {
		return guac.Errorf(guac.StatusResourceClosed, "client gone")
	}

	done := make(chan *guac.Error, 2)
	finish := func(e *guac.Error) {
		select {
		case done <- e:
		default:
		}
		cancel(e)
	}

	// guacd → browser
	go func() {
		var buf bytes.Buffer
		for {
			_ = g.SetReadDeadline(time.Now().Add(GuacdIdleTimeout))
			raw, err := g.Reader.ReadRaw()
			if err != nil {
				switch {
				case errors.Is(err, io.EOF), errors.Is(err, net.ErrClosed):
					finish(guac.Errorf(guac.StatusSuccess, "guacd_closed"))
				case isTimeout(err):
					finish(guac.Errorf(guac.StatusUpstreamTimeout, "guacd idle timeout"))
				case errors.Is(err, guac.ErrMalformed), errors.Is(err, guac.ErrTooLarge):
					finish(guac.Errorf(guac.StatusServerError, "invalid data from guacd"))
				default:
					finish(guac.Errorf(guac.StatusUpstreamError, "guacd read failed"))
				}
				return
			}
			buf.Write(raw)
			if !g.Reader.Buffered() || buf.Len() >= batchSize {
				if err := write(buf.Bytes()); err != nil {
					finish(guac.Errorf(guac.StatusResourceClosed, "client_write_failed"))
					return
				}
				buf.Reset()
			}
		}
	}()

	// browser → guacd
	go func() {
		var lastActivity time.Time
		for {
			typ, data, err := ws.Read(io0)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				if websocket.CloseStatus(err) != -1 || errors.Is(err, io.EOF) {
					finish(guac.Errorf(guac.StatusSuccess, "client_closed"))
				} else if ctx.Err() == nil {
					finish(guac.Errorf(guac.StatusClientBadRequest, "client read failed"))
				}
				return
			}
			if typ != websocket.MessageText {
				finish(guac.Errorf(guac.StatusClientBadType, "binary frames not supported"))
				return
			}
			list, err := guac.ParseAll(data, MaxClientElement)
			if err != nil {
				finish(guac.Errorf(guac.StatusClientBadRequest, "malformed client instruction"))
				return
			}
			var forward bytes.Buffer
			userInput := false
			for _, ins := range list {
				if ins.Opcode == guac.InternalOpcode {
					if len(ins.Args) > 0 && ins.Args[0] == "ping" {
						if err := write(ins.Bytes()); err != nil {
							finish(guac.Errorf(guac.StatusResourceClosed, "client_write_failed"))
							return
						}
					}
					continue
				}
				switch ins.Opcode {
				case "nop", "sync":
				default:
					userInput = true
				}
				forward.WriteString(ins.String())
			}
			if forward.Len() > 0 {
				_ = g.SetWriteDeadline(time.Now().Add(WriteTimeout))
				if _, err := g.Write(forward.Bytes()); err != nil {
					finish(guac.Errorf(guac.StatusUpstreamError, "guacd write failed"))
					return
				}
			}
			if userInput && opt.OnActivity != nil && time.Since(lastActivity) > 30*time.Second {
				lastActivity = time.Now()
				opt.OnActivity()
			}
		}
	}()

	var reason *guac.Error
	select {
	case reason = <-done:
	case <-ctx.Done():
		reason = AsGuacError(context.Cause(ctx))
	}
	_ = g.Close()
	return reason
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
