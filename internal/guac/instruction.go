// Package guac implements the Guacamole protocol: instruction encoding and
// streaming decoding, the guacd handshake and status codes.
//
// Parts of the design (streaming reader, handshake flow) are informed by
// github.com/wwt/guac (Apache-2.0); see NOTICE.
package guac

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"
)

// InternalOpcode is the empty opcode used for tunnel-internal messages
// (tunnel UUID announcement and ping).
const InternalOpcode = ""

// Instruction is a single Guacamole protocol instruction.
type Instruction struct {
	Opcode string
	Args   []string
}

// New builds an instruction.
func New(opcode string, args ...string) Instruction {
	return Instruction{Opcode: opcode, Args: args}
}

// String encodes the instruction as LENGTH.VALUE,...; where LENGTH counts
// Unicode code points.
func (i Instruction) String() string {
	var b strings.Builder
	writeElement(&b, i.Opcode)
	for _, a := range i.Args {
		b.WriteByte(',')
		writeElement(&b, a)
	}
	b.WriteByte(';')
	return b.String()
}

// Bytes returns the encoded instruction.
func (i Instruction) Bytes() []byte { return []byte(i.String()) }

func writeElement(b *strings.Builder, v string) {
	b.WriteString(strconv.Itoa(utf8.RuneCountInString(v)))
	b.WriteByte('.')
	b.WriteString(v)
}

// ErrMalformed is returned for syntactically invalid instructions.
var ErrMalformed = errors.New("guac: malformed instruction")

// ErrTooLarge is returned when an element or instruction exceeds the limits.
var ErrTooLarge = errors.New("guac: instruction too large")

// Parse decodes exactly one instruction from data (which must contain the
// full instruction including the terminating ';'). It returns the
// instruction and the number of bytes consumed.
func Parse(data []byte, maxElement int) (Instruction, int, error) {
	n, elems, err := scan(data, maxElement, true)
	if err != nil {
		return Instruction{}, 0, err
	}
	if n == 0 {
		return Instruction{}, 0, ErrMalformed
	}
	return Instruction{Opcode: elems[0], Args: elems[1:]}, n, nil
}

// ParseAll decodes every instruction in data, which must consist solely of
// complete instructions.
func ParseAll(data []byte, maxElement int) ([]Instruction, error) {
	var out []Instruction
	for len(data) > 0 {
		ins, n, err := Parse(data, maxElement)
		if err != nil {
			return nil, err
		}
		out = append(out, ins)
		data = data[n:]
	}
	return out, nil
}

// scan looks for one complete instruction at the start of data. It returns
// the byte length of the instruction (0 if incomplete). When collect is
// true it also returns the decoded elements.
func scan(data []byte, maxElement int, collect bool) (int, []string, error) {
	pos := 0
	var elems []string
	for {
		// length prefix
		start := pos
		length := 0
		for {
			if pos >= len(data) {
				return 0, nil, nil
			}
			c := data[pos]
			if c == '.' {
				break
			}
			if c < '0' || c > '9' {
				return 0, nil, ErrMalformed
			}
			length = length*10 + int(c-'0')
			if maxElement > 0 && length > maxElement {
				return 0, nil, ErrTooLarge
			}
			pos++
		}
		if pos == start {
			return 0, nil, ErrMalformed
		}
		pos++ // skip '.'
		valueStart := pos
		for r := 0; r < length; r++ {
			if pos >= len(data) {
				return 0, nil, nil
			}
			c := data[pos]
			if c < utf8.RuneSelf {
				pos++
				continue
			}
			if !utf8.FullRune(data[pos:]) {
				return 0, nil, nil
			}
			_, size := utf8.DecodeRune(data[pos:])
			pos += size
		}
		if collect {
			elems = append(elems, string(data[valueStart:pos]))
		}
		if pos >= len(data) {
			return 0, nil, nil
		}
		switch data[pos] {
		case ',':
			pos++
		case ';':
			return pos + 1, elems, nil
		default:
			return 0, nil, ErrMalformed
		}
	}
}
