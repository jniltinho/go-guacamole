package guac

import (
	"fmt"
	"strconv"
)

// Status is a Guacamole protocol status code (Guacamole.Status.Code).
type Status int

const (
	StatusSuccess             Status = 0x0000
	StatusUnsupported         Status = 0x0100
	StatusServerError         Status = 0x0200
	StatusServerBusy          Status = 0x0201
	StatusUpstreamTimeout     Status = 0x0202
	StatusUpstreamError       Status = 0x0203
	StatusResourceNotFound    Status = 0x0204
	StatusResourceConflict    Status = 0x0205
	StatusResourceClosed      Status = 0x0206
	StatusUpstreamNotFound    Status = 0x0207
	StatusUpstreamUnavailable Status = 0x0208
	StatusSessionConflict     Status = 0x0209
	StatusSessionTimeout      Status = 0x020A
	StatusSessionClosed       Status = 0x020B
	StatusClientBadRequest    Status = 0x0300
	StatusClientUnauthorized  Status = 0x0301
	StatusClientForbidden     Status = 0x0303
	StatusClientTimeout       Status = 0x0308
	StatusClientOverrun       Status = 0x030D
	StatusClientBadType       Status = 0x030F
	StatusClientTooMany       Status = 0x031D
)

// WebSocketCloseCode maps a status to the WebSocket close code used when the
// tunnel is closed for that reason.
func (s Status) WebSocketCloseCode() int {
	switch {
	case s == StatusSuccess:
		return 1000
	case s >= 0x0300 && s < 0x0400:
		return 1008
	default:
		return 1011
	}
}

// Reason is the decimal string sent as WebSocket close reason; guacamole-common-js
// parses it back into a Guacamole.Status.
func (s Status) Reason() string { return strconv.Itoa(int(s)) }

// Error is a Guacamole error carrying a status code.
type Error struct {
	Status  Status
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("guac: %s (status %d)", e.Message, e.Status)
}

// Errorf builds an *Error.
func Errorf(s Status, format string, a ...any) *Error {
	return &Error{Status: s, Message: fmt.Sprintf(format, a...)}
}

// Instruction returns the "error" instruction for this error.
func (e *Error) Instruction() Instruction {
	return New("error", e.Message, strconv.Itoa(int(e.Status)))
}
