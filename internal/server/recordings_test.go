package server

import (
	"testing"

	"github.com/jniltinho/go-guacamole/internal/config"
	"github.com/jniltinho/go-guacamole/internal/store"
	"github.com/jniltinho/go-guacamole/internal/tunnel"
)

func TestCleanTranscript(t *testing.T) {
	in := "\x1b]0;lab@host: ~\x07\x1b[01;32mlab@host\x1b[00m:~$ ls\b\bls -l\r\ntotal 0\r\n\x1b[?2004hprompt$ "
	got := cleanTranscript(in)
	want := "lab@host:~$ ls -l\ntotal 0\nprompt$\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRecordingParamsAndPolicy(t *testing.T) {
	a := &App{Cfg: &config.Config{Recording: config.RecordingConfig{Enabled: true, Path: "/record", Screen: true, Typescript: true}}}
	conn := &store.Connection{Protocol: "ssh", Parameters: map[string]string{"hostname": "h", "recording-path": "/elsewhere", "typescript-name": "x"}}
	tn := &tunnel.Tunnel{ID: "0f8fad5b-d9cb-469f-a165-70867728950e", RecVideo: true, RecText: true}
	p := a.recordingParams(conn, tn)
	if p["recording-path"] != "/record" || p["recording-name"] != tn.ID || p["typescript-name"] != tn.ID+".typescript" || p["create-recording-path"] != "true" {
		t.Fatalf("ssh params: %v", p)
	}
	if p["recording-include-keys"] != "" {
		t.Fatal("keys not requested")
	}
	if conn.Parameters["recording-path"] != "/elsewhere" {
		t.Fatal("stored connection parameters must not be mutated")
	}
	// hand-typed recording parameters never reach guacd when the policy records nothing
	if p := a.recordingParams(conn, &tunnel.Tunnel{ID: "x"}); p["recording-path"] != "" || p["typescript-name"] != "" || p["hostname"] != "h" {
		t.Fatalf("unrecorded params: %v", p)
	}
	// an exception starts the session without capture
	if p := a.recordingParams(conn, &tunnel.Tunnel{ID: "x", RecVideo: true, RecException: true}); p["recording-path"] != "" {
		t.Fatal("exception must not record")
	}
	if _, err := a.recordingFile("../../etc/passwd"); err == nil {
		t.Fatal("path traversal accepted")
	}

	on, off := "on", "off"
	d := recDefaults{Video: true, Text: true, Keys: true}
	if s := effectiveRecording(&store.Connection{Protocol: "rdp"}, d); !s.Video || s.Text || !s.Keys {
		t.Fatalf("rdp: %+v (text is SSH only)", s)
	}
	if s := effectiveRecording(&store.Connection{Protocol: "ssh", RecVideo: &off, RecKeys: &on}, d); s.Video || s.Keys || !s.Text {
		t.Fatalf("keys need video: %+v", s)
	}
	if !longer(keepForever, 30) || longer(30, keepForever) || !longer(90, 30) || longer(30, 30) {
		t.Fatal("retention ordering")
	}
}
