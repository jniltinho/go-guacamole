package guac

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestEncodeUnicode(t *testing.T) {
	got := New("name", "ação").String()
	if want := "4.name,4.ação;"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestParseRoundTrip(t *testing.T) {
	in := New("key", "65307", "1", "", "日本語,;.")
	out, n, err := Parse(in.Bytes(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(in.Bytes()) || out.Opcode != "key" || len(out.Args) != 4 || out.Args[3] != "日本語,;." {
		t.Fatalf("bad parse: %+v n=%d", out, n)
	}
}

func TestParseMalformed(t *testing.T) {
	for _, s := range []string{"x.abc;", "3.abc!", ".abc;", "3abc;"} {
		if _, _, err := Parse([]byte(s), 0); !errors.Is(err, ErrMalformed) {
			t.Errorf("%q: expected ErrMalformed, got %v", s, err)
		}
	}
}

func TestParseLimit(t *testing.T) {
	if _, _, err := Parse([]byte("100.x"), 10); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestReaderSplitReads(t *testing.T) {
	data := New("sync", "12345").String() + New("name", "ção").String()
	rd := NewReader(iotest.OneByteReader(strings.NewReader(data)), 0)
	a, err := rd.Read()
	if err != nil || a.Opcode != "sync" {
		t.Fatalf("first: %+v %v", a, err)
	}
	b, err := rd.Read()
	if err != nil || b.Args[0] != "ção" {
		t.Fatalf("second: %+v %v", b, err)
	}
	if _, err := rd.Read(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestReaderPartialAtEOF(t *testing.T) {
	rd := NewReader(strings.NewReader("4.sync,3.12"), 0)
	if _, err := rd.Read(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected ErrUnexpectedEOF, got %v", err)
	}
}

func TestReaderMalformed(t *testing.T) {
	rd := NewReader(strings.NewReader("a.sync;"), 0)
	if _, err := rd.Read(); !errors.Is(err, ErrMalformed) {
		t.Fatalf("expected ErrMalformed, got %v", err)
	}
}

func TestReaderTooLarge(t *testing.T) {
	rd := NewReader(strings.NewReader("9999999.x"+strings.Repeat("a", 70000)), 64<<10)
	if _, err := rd.Read(); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestReaderBuffered(t *testing.T) {
	rd := NewReader(bytes.NewReader([]byte("3.nop;3.nop;")), 0)
	if _, err := rd.ReadRaw(); err != nil {
		t.Fatal(err)
	}
	if !rd.Buffered() {
		t.Fatal("expected second instruction buffered")
	}
}

func TestNegotiateVersion(t *testing.T) {
	cases := map[string]string{
		"VERSION_1_3_0": "VERSION_1_3_0",
		"VERSION_1_5_0": "VERSION_1_5_0",
		"VERSION_1_6_0": "VERSION_1_5_0",
	}
	for in, want := range cases {
		if got := NegotiateVersion(in); got != want {
			t.Errorf("%s: got %s want %s", in, got, want)
		}
	}
}

func TestStatusClose(t *testing.T) {
	if StatusUpstreamUnavailable != 520 || StatusClientForbidden != 771 || StatusClientTooMany != 797 {
		t.Fatal("status constants wrong")
	}
	if StatusUpstreamUnavailable.WebSocketCloseCode() != 1011 || StatusClientForbidden.WebSocketCloseCode() != 1008 {
		t.Fatal("close codes wrong")
	}
}

// fakeGuacd runs a scripted guacd on one end of a pipe.
func fakeGuacd(t *testing.T, offer []string, reply func(rd *Reader, w net.Conn)) net.Conn {
	t.Helper()
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		rd := NewReader(server, 0)
		sel, err := rd.Read()
		if err != nil || sel.Opcode != "select" {
			return
		}
		_, _ = server.Write(New("args", offer...).Bytes())
		reply(rd, server)
	}()
	return client
}

func TestHandshake(t *testing.T) {
	got := make(chan []Instruction, 1)
	c := fakeGuacd(t, []string{"VERSION_1_5_0", "hostname", "port", "password"}, func(rd *Reader, w net.Conn) {
		var list []Instruction
		for {
			ins, err := rd.Read()
			if err != nil {
				return
			}
			list = append(list, ins)
			if ins.Opcode == "connect" {
				got <- list
				_, _ = w.Write(New("ready", "$abc").Bytes())
				return
			}
		}
	})
	conn, err := Handshake(context.Background(), c, Config{
		Protocol:   "ssh",
		Parameters: map[string]string{"hostname": "10.0.0.1", "port": "22"},
		Client:     ClientInfo{Width: 800, Height: 600, DPI: 96, Image: []string{"image/png"}, Timezone: "America/Sao_Paulo"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if conn.ConnectionID != "$abc" || conn.Version != "VERSION_1_5_0" {
		t.Fatalf("bad conn %+v", conn)
	}
	list := <-got
	ops := []string{}
	for _, i := range list {
		ops = append(ops, i.Opcode)
	}
	if strings.Join(ops, ",") != "size,audio,video,image,timezone,connect" {
		t.Fatalf("bad order %v", ops)
	}
	connect := list[len(list)-1]
	if strings.Join(connect.Args, "|") != "VERSION_1_5_0|10.0.0.1|22|" {
		t.Fatalf("bad connect args %v", connect.Args)
	}
}

func TestHandshakeError(t *testing.T) {
	c := fakeGuacd(t, []string{"VERSION_1_5_0", "hostname"}, func(rd *Reader, w net.Conn) {
		for {
			ins, err := rd.Read()
			if err != nil {
				return
			}
			if ins.Opcode == "connect" {
				_, _ = w.Write(New("error", "Upstream not found", "519").Bytes())
				return
			}
		}
	})
	_, err := Handshake(context.Background(), c, Config{Protocol: "ssh"})
	var ge *Error
	if !errors.As(err, &ge) || ge.Status != StatusUpstreamNotFound {
		t.Fatalf("expected status 519, got %v", err)
	}
}

func TestHandshakeCancel(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	go func() { _, _ = io.Copy(io.Discard, server) }()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	_, err := Handshake(ctx, client, Config{Protocol: "ssh"})
	var ge *Error
	if !errors.As(err, &ge) || ge.Status != StatusResourceClosed {
		t.Fatalf("expected cancellation error, got %v", err)
	}
}

func TestDialUnavailable(t *testing.T) {
	_, err := Dial(context.Background(), DialOptions{Addr: "127.0.0.1:1"})
	var ge *Error
	if !errors.As(err, &ge) || ge.Status != StatusUpstreamUnavailable {
		t.Fatalf("expected 520, got %v", err)
	}
}
