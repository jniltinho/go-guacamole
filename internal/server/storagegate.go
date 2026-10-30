package server

import (
	"errors"
	"os"
	"time"

	"github.com/jniltinho/go-guacamole/internal/store"
)

// guacd gives no "file closed" signal: a recording set is uploaded only after the session ended and none of its
// files changed for QuietPeriod, and its local copy is removed only LocalRemovalWindow after the session ended
// (openspec add-async-jobs-object-storage, design D5).
const (
	QuietPeriod        = 2 * time.Minute
	LocalRemovalWindow = time.Hour
)

var errSetChanging = errors.New("recording files changed recently")

// setQuiet reports whether every file of a set exists and was last modified at least QuietPeriod before now.
func setQuiet(files []string, now time.Time) error {
	if len(files) == 0 {
		return errors.New("recording set has no files")
	}
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			return err
		}
		if now.Sub(st.ModTime()) < QuietPeriod {
			return errSetChanging
		}
	}
	return nil
}

// uploadReady: the session ended and its files are quiet.
func uploadReady(h *store.HistoryEntry, files []string, now time.Time) error {
	if h.EndAt == nil {
		return errors.New("session still active")
	}
	return setQuiet(files, now)
}

// localRemovalAllowed: ended long enough ago and not under legal hold (the claim and the re-hash are checked by the
// caller that removes the files).
func localRemovalAllowed(h *store.HistoryEntry, now time.Time) bool {
	return h.EndAt != nil && h.HoldAt == nil && now.UnixMilli()-*h.EndAt >= LocalRemovalWindow.Milliseconds()
}
