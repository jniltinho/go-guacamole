//go:build noobjstore

package objstore

import (
	"context"
	"io"
	"log/slog"
)

// Built with -tags noobjstore: rclone is not linked and every operation reports ErrUnavailable.
const available = false

// Setup does nothing without object storage.
func Setup(*slog.Logger) {}

// Remote is never connected in this build.
type Remote struct{}

// Connect always fails with ErrUnavailable.
func Connect(context.Context, Target, Policy) (*Remote, error) { return nil, ErrUnavailable }

func (*Remote) Put(context.Context, string, string) (Stored, error) { return Stored{}, ErrUnavailable }
func (*Remote) Verify(context.Context, string, int64, string) error { return ErrUnavailable }
func (*Remote) Open(context.Context, string) (io.ReadCloser, int64, error) {
	return nil, 0, ErrUnavailable
}
func (*Remote) DeleteAll(context.Context, string) (int, error)       { return 0, ErrUnavailable }
func (*Remote) PruneVersions(context.Context, string) (int, error)   { return 0, ErrUnavailable }
func (*Remote) HasOldVersions(context.Context, string) (bool, error) { return false, ErrUnavailable }
func (*Remote) List(context.Context, string) ([]Entry, error)        { return nil, ErrUnavailable }
func (*Remote) Probe(context.Context, string) (ProbeResult, error) {
	return ProbeResult{}, ErrUnavailable
}
func (*Remote) DeleteSet(context.Context, []string) (int, error) { return 0, ErrUnavailable }
