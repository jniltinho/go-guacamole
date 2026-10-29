package server

import (
	"testing"

	"github.com/jniltinho/go-guacamole/internal/config"
	"github.com/jniltinho/go-guacamole/internal/store"
)

func TestCleanTranscript(t *testing.T) {
	in := "\x1b]0;lab@host: ~\x07\x1b[01;32mlab@host\x1b[00m:~$ ls\b\bls -l\r\ntotal 0\r\n\x1b[?2004hprompt$ "
	got := cleanTranscript(in)
	want := "lab@host:~$ ls -l\ntotal 0\nprompt$\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRecordingParams(t *testing.T) {
	a := &App{Cfg: &config.Config{Recording: config.RecordingConfig{Enabled: true, Path: "/record", Screen: true, Typescript: true}}}
	conn := &store.Connection{Protocol: "ssh", Parameters: map[string]string{"hostname": "h", "recording-path": "/elsewhere"}}
	p, rec, ts := a.recordingParams(conn, "0f8fad5b-d9cb-469f-a165-70867728950e")
	if rec == "" || ts != rec+".typescript" || p["recording-path"] != "/record" || p["typescript-name"] != ts || p["create-recording-path"] != "true" {
		t.Fatalf("ssh params: %v %s %s", p, rec, ts)
	}
	if conn.Parameters["recording-path"] != "/elsewhere" {
		t.Fatal("stored connection parameters must not be mutated")
	}
	_, _, ts = a.recordingParams(&store.Connection{Protocol: "rdp", Parameters: map[string]string{}}, "x")
	if ts != "" {
		t.Fatal("typescript only for terminal protocols")
	}
	a.Cfg.Recording.Enabled = false
	if p, rec, _ := a.recordingParams(conn, "x"); rec != "" || p["recording-path"] != "/elsewhere" {
		t.Fatal("disabled recording must not change parameters")
	}
	if _, err := a.recordingFile("../../etc/passwd"); err == nil {
		t.Fatal("path traversal accepted")
	}
}
