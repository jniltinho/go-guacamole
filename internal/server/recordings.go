package server

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/jniltinho/go-guacamole/internal/store"
)

// recordingName matches the file names goguacamole asks guacd to create.
var recordingName = regexp.MustCompile(`^[0-9a-f-]{36}(\.typescript)?$`)

// ansiSeq matches terminal control sequences (CSI, OSC, charset selection, single ESC codes).
var ansiSeq = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\x1b[()][0-9A-Za-z]|\x1b[=>78DEHMNOcZ]`)

// recordingFile resolves a stored recording name inside the recordings directory.
func (a *App) recordingFile(name string) (string, error) {
	if name == "" || !recordingName.MatchString(name) {
		return "", errNotFound
	}
	p := filepath.Join(a.Cfg.Recording.Dir(), filepath.Base(name))
	if _, err := os.Stat(p); err != nil {
		return "", errNotFound
	}
	return p, nil
}

func (a *App) historyEntry(c *echo.Context) (*store.HistoryEntry, error) {
	if err := requirePerm(c, store.PermAudit); err != nil {
		return nil, err
	}
	id, err := pathID(c, "id")
	if err != nil {
		return nil, err
	}
	return a.Store.GetHistory(c.Request().Context(), id)
}

// getRecording streams the guacd screen recording (Guacamole protocol dump)
// for playback with Guacamole.SessionRecording.
func (a *App) getRecording(c *echo.Context) error {
	h, err := a.historyEntry(c)
	if err != nil {
		return err
	}
	p, err := a.recordingFile(h.Recording)
	if err != nil {
		return err
	}
	f, err := os.Open(p)
	if err != nil {
		return errNotFound
	}
	defer f.Close()
	a.auditBy(c, "recording.view", "history", idStr(h.ID), h.ConnectionName, map[string]any{"user": h.Username, "kind": "screen"})
	c.Response().Header().Set("Cache-Control", "private, no-store")
	c.Response().Header().Set("Content-Disposition", `inline; filename="`+h.Recording+`.guac"`)
	return c.Stream(http.StatusOK, "application/octet-stream", f)
}

// getTypescript returns the SSH text transcript: format=text strips terminal
// control sequences, format=raw returns the typescript as written by guacd
// (replayable with `scriptreplay <file>.timing <file>`).
func (a *App) getTypescript(c *echo.Context) error {
	h, err := a.historyEntry(c)
	if err != nil {
		return err
	}
	p, err := a.recordingFile(h.Typescript)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return errNotFound
	}
	a.auditBy(c, "recording.view", "history", idStr(h.ID), h.ConnectionName, map[string]any{"user": h.Username, "kind": "typescript"})
	c.Response().Header().Set("Cache-Control", "private, no-store")
	if c.QueryParam("format") == "raw" {
		c.Response().Header().Set("Content-Disposition", `attachment; filename="`+h.Typescript+`"`)
		return c.Blob(http.StatusOK, "application/octet-stream", raw)
	}
	return c.Blob(http.StatusOK, "text/plain; charset=utf-8", []byte(cleanTranscript(string(raw))))
}

// cleanTranscript removes terminal control sequences and resolves carriage
// returns / backspaces so the transcript reads like the terminal looked.
func cleanTranscript(s string) string {
	s = ansiSeq.ReplaceAllString(s, "")
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, l := range lines {
		if j := strings.LastIndex(l, "\r"); j >= 0 {
			l = l[j+1:]
		}
		var b []rune
		for _, r := range l {
			switch {
			case r == '\b':
				if len(b) > 0 {
					b = b[:len(b)-1]
				}
			case r == '\t' || r >= 0x20 && r != 0x7f:
				b = append(b, r)
			}
		}
		lines[i] = strings.TrimRight(string(b), " ")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}
