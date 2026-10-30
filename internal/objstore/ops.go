//go:build !noobjstore

package objstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/object"
	"github.com/rclone/rclone/fs/walk"
	"github.com/rclone/rclone/lib/version"
)

// Remote is an opened target. For the S3 family a second view lists every version (and delete marker) as an object
// of its own, so deleting it removes that exact version; GCS and OCI native expose current objects only (their
// buckets must not keep old versions: Test reports it).
type Remote struct {
	f        fs.Fs
	ctx      context.Context
	versions fs.Fs // nil when the provider cannot list versions
}

// Connect opens the target t under the endpoint policy p.
func Connect(ctx context.Context, t Target, p Policy) (*Remote, error) {
	f, fctx, err := Open(ctx, t, p)
	if err != nil {
		return nil, err
	}
	r := &Remote{f: f, ctx: fctx}
	if t.Provider == AWS || t.Provider == S3 || t.Provider == OCI {
		v := t
		v.withVersions = true
		if r.versions, _, err = Open(ctx, v, p); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Put streams a local file to key and verifies it by reading the object back (SHA-256 and size). Provider checksums
// are not trusted: S3 keeps client-supplied MD5s for multipart uploads.
func (r *Remote) Put(ctx context.Context, key, local string) (Stored, error) {
	ctx = r.with(ctx)
	f, err := os.Open(local)
	if err != nil {
		return Stored{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Stored{}, err
	}
	h := sha256.New()
	info := object.NewStaticObjectInfo(key, st.ModTime(), st.Size(), true, nil, r.f)
	if _, err := r.f.Put(ctx, io.TeeReader(f, h), info); err != nil {
		return Stored{}, errors.New(Clean(err.Error()))
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if err := r.Verify(ctx, key, st.Size(), sum); err != nil {
		return Stored{}, err
	}
	return Stored{Key: key, Size: st.Size(), SHA256: sum}, nil
}

// Verify reads key back and compares its size and SHA-256.
func (r *Remote) Verify(ctx context.Context, key string, size int64, sum string) error {
	rc, n, err := r.Open(ctx, key)
	if err != nil {
		return err
	}
	defer rc.Close()
	h := sha256.New()
	got, err := io.Copy(h, rc)
	if err != nil {
		return errors.New(Clean(err.Error()))
	}
	if got != size || n != size || hex.EncodeToString(h.Sum(nil)) != sum {
		return fmt.Errorf("%w: %s", ErrMismatch, key)
	}
	return nil
}

// Open streams the current version of key, with its size.
func (r *Remote) Open(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	ctx = r.with(ctx)
	o, err := r.f.NewObject(ctx, key)
	if err != nil {
		return nil, 0, errors.New(Clean(err.Error()))
	}
	rc, err := o.Open(ctx)
	if err != nil {
		return nil, 0, errors.New(Clean(err.Error()))
	}
	return rc, o.Size(), nil
}

// all lists every object under dir in the versions view (or the current view), recursively.
func (r *Remote) all(ctx context.Context, dir string) ([]fs.Object, error) {
	f := r.f
	if r.versions != nil {
		f = r.versions
	}
	var out []fs.Object
	err := walk.ListR(r.with(ctx), f, dir, true, -1, walk.ListObjects, func(entries fs.DirEntries) error {
		entries.ForObject(func(o fs.Object) { out = append(out, o) })
		return nil
	})
	if errors.Is(err, fs.ErrorDirNotFound) {
		return nil, nil
	}
	return out, err
}

// DeleteAll removes every version of every object under dir and confirms that none remains. Refusals (object
// lock, retention, permissions) are returned so the caller keeps the deletion pending.
func (r *Remote) DeleteAll(ctx context.Context, dir string) (int, error) {
	objs, err := r.all(ctx, dir)
	if err != nil {
		return 0, errors.New(Clean(err.Error()))
	}
	n := 0
	for _, o := range objs {
		if err := o.Remove(r.with(ctx)); err != nil {
			return n, fmt.Errorf("delete %s: %s", o.Remote(), Clean(err.Error()))
		}
		n++
	}
	left, err := r.all(ctx, dir)
	if err != nil {
		return n, errors.New(Clean(err.Error()))
	}
	if len(left) > 0 {
		return n, fmt.Errorf("%d versions remain under %s", len(left), dir)
	}
	return n, nil
}

// HasOldVersions reports whether older versions (or delete markers) exist under dir.
func (r *Remote) HasOldVersions(ctx context.Context, dir string) (bool, error) {
	if r.versions == nil {
		return false, nil
	}
	objs, err := r.all(ctx, dir)
	if err != nil {
		return false, errors.New(Clean(err.Error()))
	}
	for _, o := range objs {
		if version.Match(path.Base(o.Remote())) {
			return true, nil
		}
	}
	return false, nil
}

// PruneVersions removes the older versions (and delete markers) left under dir by earlier attempts; the current
// objects, verified by the caller, stay.
func (r *Remote) PruneVersions(ctx context.Context, dir string) (int, error) {
	if r.versions == nil {
		return 0, nil
	}
	objs, err := r.all(ctx, dir)
	if err != nil {
		return 0, errors.New(Clean(err.Error()))
	}
	n := 0
	for _, o := range objs {
		if !version.Match(path.Base(o.Remote())) {
			continue // current version
		}
		if err := o.Remove(r.with(ctx)); err != nil {
			return n, fmt.Errorf("delete %s: %s", o.Remote(), Clean(err.Error()))
		}
		n++
	}
	return n, nil
}

// List returns the current objects under dir.
func (r *Remote) List(ctx context.Context, dir string) ([]Entry, error) {
	var out []Entry
	err := walk.ListR(r.with(ctx), r.f, dir, true, -1, walk.ListObjects, func(entries fs.DirEntries) error {
		entries.ForObject(func(o fs.Object) {
			out = append(out, Entry{Key: o.Remote(), Size: o.Size(), ModTime: o.ModTime(r.ctx)})
		})
		return nil
	})
	if errors.Is(err, fs.ErrorDirNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New(Clean(err.Error()))
	}
	return out, nil
}

// with carries the target's rclone configuration (egress proxy, retries) into ctx.
func (r *Remote) with(ctx context.Context) context.Context {
	if r.ctx == nil {
		return ctx
	}
	ci := *fs.GetConfig(r.ctx)
	nctx, nci := fs.AddConfig(ctx)
	*nci = ci
	return nctx
}

// Probe writes a test object under dir, reads it back, deletes it and reports what it learned: whether the bucket
// keeps old versions (a delete leaves a version or a marker behind) and whether deletion is refused (object lock or
// retention). The probe never touches recordings.
func (r *Remote) Probe(ctx context.Context, dir string) (ProbeResult, error) {
	var res ProbeResult
	tmp, err := os.CreateTemp("", "gg-probe-*")
	if err != nil {
		return res, err
	}
	defer os.Remove(tmp.Name())
	_, _ = tmp.WriteString("goguacamole storage test " + time.Now().UTC().Format(time.RFC3339Nano))
	tmp.Close()
	key := path.Join(dir, "probe")
	if _, err := r.Put(ctx, key, tmp.Name()); err != nil {
		return res, err
	}
	if o, err := r.f.NewObject(r.with(ctx), key); err == nil {
		_ = o.Remove(r.with(ctx)) // current version only
	}
	if r.versions != nil {
		left, _ := r.all(ctx, dir)
		res.Versioned = len(left) > 0
	}
	if _, err := r.DeleteAll(ctx, dir); err != nil {
		res.DeleteRefused = true
		return res, fmt.Errorf("the test object was written but cannot be deleted (object lock or retention?): %w", err)
	}
	return res, nil
}

// DeleteSet removes every version of exactly the given keys (one recording set; its sibling set in the same
// directory, and anything in subdirectories, is left alone) and confirms that none remains.
func (r *Remote) DeleteSet(ctx context.Context, keys []string) (int, error) {
	want, dirs := map[string]bool{}, map[string]bool{}
	for _, k := range keys {
		want[k], dirs[path.Dir(k)] = true, true
	}
	mine := func(o fs.Object) bool {
		_, base := version.Remove(path.Base(o.Remote()))
		return want[path.Join(path.Dir(o.Remote()), base)]
	}
	n := 0
	for dir := range dirs {
		objs, err := r.all(ctx, dir)
		if err != nil {
			return n, errors.New(Clean(err.Error()))
		}
		for _, o := range objs {
			if !mine(o) {
				continue
			}
			if err := o.Remove(r.with(ctx)); err != nil {
				return n, fmt.Errorf("delete %s: %s", o.Remote(), Clean(err.Error()))
			}
			n++
		}
		left, err := r.all(ctx, dir)
		if err != nil {
			return n, errors.New(Clean(err.Error()))
		}
		for _, o := range left {
			if mine(o) {
				return n, fmt.Errorf("versions of %s remain", path.Base(o.Remote()))
			}
		}
	}
	return n, nil
}
