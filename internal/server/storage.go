package server

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/jniltinho/go-guacamole/internal/jobs"
	"github.com/jniltinho/go-guacamole/internal/objstore"
	"github.com/jniltinho/go-guacamole/internal/store"
)

// Recording storage settings (openspec add-async-jobs-object-storage, spec recording-storage): local only, or
// local + an object storage target. Target secrets are write-only; a change of location creates a new target version
// and the previous one stays readable for the recordings it holds.

const settingStorage = "recording.storage"

type storageSettings struct {
	Enabled   bool  `json:"enabled"`   // upload recordings to the target
	KeepLocal bool  `json:"keepLocal"` // keep local files after a verified upload (until retention)
	TargetID  int64 `json:"targetId,string"`
}

type storageView struct {
	storageSettings
	Target      *objstore.Target `json:"target"`
	SecretsSet  map[string]bool  `json:"secretsSet"`
	Available   bool             `json:"available"`
	Unavailable string           `json:"unavailableReason,omitempty"`
	Queue       jobs.Health      `json:"queue"`
}

type remoteCache struct {
	mu sync.Mutex
	m  map[int64]*objstore.Remote
}

func (a *App) storageSettings(ctx context.Context) (storageSettings, error) {
	s := storageSettings{KeepLocal: true}
	err := a.Store.GetSetting(ctx, settingStorage, &s)
	return s, err
}

func (a *App) storageTarget(ctx context.Context, id int64) (objstore.Target, error) {
	var t objstore.Target
	err := a.Store.GetStorageTarget(ctx, id, &t)
	return t, err
}

func (a *App) storagePolicy() objstore.Policy {
	return objstore.Policy{AllowHTTP: a.Cfg.ObjStore.AllowHTTP, Allowed: a.Cfg.ObjStore.AllowedEndpoints}
}

// storageAvailable: object storage is compiled in and the job queue is up.
func (a *App) storageAvailable() (bool, string) {
	if !objstore.Available {
		return false, "this build has no object storage (noobjstore)"
	}
	if h := a.queueHealth(); h.State != jobs.Up {
		return false, "object storage needs the job queue (Redis), which is " + string(h.State)
	}
	return true, ""
}

// remote returns the connected target version id (cached until its settings change).
func (a *App) remote(ctx context.Context, id int64) (*objstore.Remote, error) {
	a.remotes.mu.Lock()
	defer a.remotes.mu.Unlock()
	if r := a.remotes.m[id]; r != nil {
		return r, nil
	}
	t, err := a.storageTarget(ctx, id)
	if err != nil {
		return nil, err
	}
	r, err := objstore.Connect(context.WithoutCancel(ctx), t, a.storagePolicy())
	if err != nil {
		return nil, err
	}
	if a.remotes.m == nil {
		a.remotes.m = map[int64]*objstore.Remote{}
	}
	a.remotes.m[id] = r
	return r, nil
}

func (a *App) forgetRemote(id int64) {
	a.remotes.mu.Lock()
	delete(a.remotes.m, id)
	a.remotes.mu.Unlock()
}

func secretFields(t *objstore.Target) map[string]*string {
	return map[string]*string{"secretKey": &t.SecretKey, "serviceAccount": &t.ServiceAccount}
}

// sameLocation reports whether two targets point at the same place (otherwise a new target version is created).
func sameLocation(x, y objstore.Target) bool {
	return x.Provider == y.Provider && x.Endpoint == y.Endpoint && x.Region == y.Region && x.Namespace == y.Namespace &&
		x.Bucket == y.Bucket && strings.Trim(x.Prefix, "/") == strings.Trim(y.Prefix, "/") && x.PathStyle == y.PathStyle &&
		x.OCIAuth == y.OCIAuth && x.Compartment == y.Compartment
}

// publicTarget is the target without secret values.
func publicTarget(t objstore.Target) (objstore.Target, map[string]bool) {
	set := map[string]bool{}
	for name, p := range secretFields(&t) {
		set[name] = *p != ""
		*p = ""
	}
	return t, set
}

func (a *App) getStorageSettings(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	s, err := a.storageSettings(ctx)
	if err != nil {
		return err
	}
	ok, why := a.storageAvailable()
	v := storageView{storageSettings: s, SecretsSet: map[string]bool{}, Available: ok, Unavailable: why, Queue: a.queueHealth()}
	if s.TargetID != 0 {
		t, err := a.storageTarget(ctx, s.TargetID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		pt, set := publicTarget(t)
		v.Target, v.SecretsSet = &pt, set
	}
	return c.JSON(http.StatusOK, v)
}

type storageInput struct {
	Enabled   bool             `json:"enabled"`
	KeepLocal bool             `json:"keepLocal"`
	Target    *objstore.Target `json:"target"` // nil or empty provider: local only
	Clear     []string         `json:"clear"`  // secrets removed on purpose
}

// mergeTarget fills omitted secrets from the current target and validates.
func (a *App) mergeTarget(ctx context.Context, cur storageSettings, in *objstore.Target, clear []string) error {
	in.Bucket, in.Prefix = strings.TrimSpace(in.Bucket), strings.Trim(strings.TrimSpace(in.Prefix), "/")
	if cur.TargetID != 0 {
		if old, err := a.storageTarget(ctx, cur.TargetID); err == nil && old.Provider == in.Provider {
			for name, p := range secretFields(in) {
				if *p == "" && !slices.Contains(clear, name) {
					*p = *secretFields(&old)[name]
				}
			}
		}
	}
	if err := in.Validate(); err != nil {
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "VALIDATION", Message: err.Error(), Field: "target"}
	}
	return nil
}

func (a *App) putStorageSettings(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	var in storageInput
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	ctx := c.Request().Context()
	cur, err := a.storageSettings(ctx)
	if err != nil {
		return err
	}
	next := storageSettings{Enabled: in.Enabled, KeepLocal: in.KeepLocal, TargetID: cur.TargetID}
	localOnly := in.Target == nil || in.Target.Provider == ""
	if in.Enabled {
		if localOnly {
			return &APIError{Status: http.StatusUnprocessableEntity, Code: "VALIDATION", Message: "choose a storage target to enable uploads", Field: "target"}
		}
		if ok, why := a.storageAvailable(); !ok {
			return apiErr(http.StatusConflict, "STORAGE_UNAVAILABLE", why)
		}
	}
	var before, after map[string]any
	if cur.TargetID != 0 {
		if t, err := a.storageTarget(ctx, cur.TargetID); err == nil {
			before = targetAudit(t)
		}
	}
	if localOnly {
		// the target is kept (retired) so recordings it holds stay readable
		if cur.TargetID != 0 {
			if err := a.Store.RetireStorageTarget(ctx, cur.TargetID); err != nil {
				return err
			}
		}
		next.TargetID = 0
	} else {
		t := *in.Target
		if err := a.mergeTarget(ctx, cur, &t, in.Clear); err != nil {
			return err
		}
		old, oldErr := a.storageTarget(ctx, cur.TargetID)
		if cur.TargetID != 0 && oldErr == nil && sameLocation(old, t) {
			if err := a.Store.UpdateStorageTarget(ctx, cur.TargetID, t); err != nil {
				return err
			}
		} else {
			id, err := a.Store.CreateStorageTarget(ctx, t)
			if err != nil {
				return err
			}
			if cur.TargetID != 0 {
				if err := a.Store.RetireStorageTarget(ctx, cur.TargetID); err != nil {
					return err
				}
			}
			next.TargetID = id
		}
		a.forgetRemote(cur.TargetID)
		after = targetAudit(t)
	}
	if err := a.Store.PutSetting(ctx, settingStorage, &next); err != nil {
		return err
	}
	a.auditBy(c, "settings.recording.storage.update", "settings", settingStorage, "recording storage", map[string]any{
		"before": before, "after": after, "enabled": next.Enabled, "keepLocal": next.KeepLocal,
		"targetId": idStr(next.TargetID), "previousTargetId": idStr(cur.TargetID)})
	return a.getStorageSettings(c)
}

// targetAudit lists a target's non-secret fields.
func targetAudit(t objstore.Target) map[string]any {
	return map[string]any{"provider": t.Provider, "bucket": t.Bucket, "prefix": t.Prefix, "region": t.Region,
		"endpoint": t.Endpoint, "namespace": t.Namespace, "sse": t.SSE, "kmsKeyId": t.KMSKeyID, "ociAuth": t.OCIAuth}
}

// testStorage checks the target in the form (saved or not): write, read back, delete a probe object.
func (a *App) testStorage(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	var in storageInput
	if err := c.Bind(&in); err != nil || in.Target == nil {
		return errBadRequest
	}
	if !objstore.Available {
		return apiErr(http.StatusConflict, "STORAGE_UNAVAILABLE", "this build has no object storage (noobjstore)")
	}
	ctx := c.Request().Context()
	cur, err := a.storageSettings(ctx)
	if err != nil {
		return err
	}
	t := *in.Target
	if err := a.mergeTarget(ctx, cur, &t, in.Clear); err != nil {
		return err
	}
	inst, err := a.Store.InstallationID(ctx)
	if err != nil {
		return err
	}
	tctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out := map[string]any{"ok": false}
	r, err := objstore.Connect(tctx, t, a.storagePolicy())
	if err == nil {
		var res objstore.ProbeResult
		res, err = r.Probe(tctx, inst+"/.goguacamole-test-"+store.RandomHex(4))
		out["versioned"], out["deleteRefused"] = res.Versioned, res.DeleteRefused
	}
	if err != nil {
		out["message"] = objstore.Clean(err.Error())
	} else {
		out["ok"] = true
	}
	a.auditBy(c, "settings.recording.storage.test", "settings", settingStorage, "recording storage",
		map[string]any{"ok": out["ok"], "target": targetAudit(t)})
	return c.JSON(http.StatusOK, out)
}

// backfillStorage uploads now every recording not in storage yet, failed ones included (attempts start over).
func (a *App) backfillStorage(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	ctx := c.Request().Context()
	set, err := a.storageSettings(ctx)
	if err != nil {
		return err
	}
	if !set.Enabled || set.TargetID == 0 {
		return apiErr(http.StatusConflict, "STORAGE_DISABLED", "enable uploads first")
	}
	if ok, why := a.storageAvailable(); !ok {
		return apiErr(http.StatusConflict, "STORAGE_UNAVAILABLE", why)
	}
	retried, err := a.Store.RetryFailedUploads(ctx, 0)
	if err != nil {
		return err
	}
	queued, err := a.queueOwed(ctx, a.Store.Now())
	if err != nil {
		return err
	}
	a.auditBy(c, "settings.recording.storage.backfill", "settings", settingStorage, "recording storage",
		map[string]any{"queued": queued, "retried": retried})
	return c.JSON(http.StatusOK, map[string]any{"queued": queued, "retried": retried})
}

// retryStorage gives a history row's failed sets a fresh count of attempts and queues them now.
func (a *App) retryStorage(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	id, err := pathID(c, "id")
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	h, err := a.Store.GetHistory(ctx, id)
	if err != nil {
		return err
	}
	if ok, why := a.storageAvailable(); !ok {
		return apiErr(http.StatusConflict, "STORAGE_UNAVAILABLE", why)
	}
	n, err := a.Store.RetryFailedUploads(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return apiErr(http.StatusConflict, "NOTHING_TO_RETRY", "no failed upload for this session")
	}
	for _, video := range []bool{true, false} {
		if err := a.enqueueUpload(ctx, store.SetRef{ID: id, Video: video}, 0); err != nil {
			return err
		}
	}
	a.auditBy(c, "recording.storage.retry", "history", idStr(id), h.ConnectionName, map[string]any{"user": h.Username})
	return c.NoContent(http.StatusNoContent)
}
