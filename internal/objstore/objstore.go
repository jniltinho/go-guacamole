//go:build !noobjstore

package objstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	_ "github.com/rclone/rclone/backend/googlecloudstorage"
	_ "github.com/rclone/rclone/backend/oracleobjectstorage"
	_ "github.com/rclone/rclone/backend/s3"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configmap"
)

// backend returns the rclone backend name and options for t.
func (t Target) backend() (string, configmap.Simple) {
	m := configmap.Simple{}
	switch t.Provider {
	case AWS, S3, OCI:
		m["provider"] = map[string]string{AWS: "AWS", S3: "Other", OCI: "Other"}[t.Provider]
		m["region"] = t.Region
		if m["region"] == "" {
			m["region"] = "us-east-1" // S3-compatible services ignore it; empty would make the SDK probe instance metadata
		}
		m["access_key_id"], m["secret_access_key"] = t.AccessKey, t.SecretKey
		m["env_auth"] = fmt.Sprint(t.AccessKey == "" && t.Provider == AWS) // AWS instance roles when no keys
		switch t.Provider {
		case S3:
			m["endpoint"], m["force_path_style"] = t.Endpoint, fmt.Sprint(t.PathStyle)
		case OCI:
			m["endpoint"] = fmt.Sprintf("https://%s.compat.objectstorage.%s.oraclecloud.com", t.Namespace, t.Region)
			m["force_path_style"] = "true"
		}
		if t.SSE != "" {
			m["server_side_encryption"] = t.SSE
			if t.SSE == "aws:kms" && t.KMSKeyID != "" {
				m["sse_kms_key_id"] = t.KMSKeyID
			}
		}
		m["no_check_bucket"] = "true" // the bucket exists; do not need CreateBucket permission
		if t.withVersions {
			m["versions"], m["version_deleted"] = "true", "true"
		}
		return "s3", m
	case OCINative:
		m["provider"], m["namespace"], m["compartment"], m["region"] = t.OCIAuth, t.Namespace, t.Compartment, t.Region
		return "oracleobjectstorage", m
	case GCS:
		m["service_account_credentials"], m["bucket_policy_only"] = t.ServiceAccount, "true"
		return "googlecloudstorage", m
	}
	return "", m
}

const available = true

var (
	setupOnce sync.Once
	egressMu  sync.Mutex
	proxies   = map[string]*egress{} // per policy
	logOut    = slog.Default()
)

// Setup installs rclone's in-memory configuration and sanitized logging; call once at startup, before any backend.
func Setup(log *slog.Logger) {
	setupOnce.Do(func() {
		_ = config.SetConfigPath("") // in-memory configuration only: no rclone.conf is read or written
		installLogger(log)
		ci := fs.GetConfig(context.Background())
		ci.LogLevel = fs.LogLevelError
	})
}

// installLogger routes rclone's logs through the sanitizer to log (the default logger when nil).
func installLogger(log *slog.Logger) {
	if log != nil {
		logOut = log
	}
	fs.SetLogger(handler{next: logOut.Handler(), clean: func(s string) string {
		secretsMu.Lock()
		defer secretsMu.Unlock()
		return known.Clean(s)
	}})
}

// context returns ctx with rclone configuration routing traffic through the egress proxy of policy p.
func egressContext(ctx context.Context, p Policy) (context.Context, error) {
	key := fmt.Sprint(p.AllowHTTP, p.Allowed)
	egressMu.Lock()
	e := proxies[key]
	if e == nil {
		var err error
		if e, err = startEgress(p); err != nil {
			egressMu.Unlock()
			return nil, err
		}
		proxies[key] = e
	}
	egressMu.Unlock()
	ctx, ci := fs.AddConfig(ctx)
	ci.HTTPProxy = e.url() // also means environment proxies are never used
	ci.LogLevel = fs.LogLevelError
	ci.LowLevelRetries = 3 // the job queue retries whole uploads; a wrong endpoint fails fast in Test
	return ctx, nil
}

// Open builds the rclone filesystem of a target (rooted at bucket/prefix) under the endpoint policy.
func Open(ctx context.Context, t Target, p Policy) (fs.Fs, context.Context, error) {
	if err := t.Validate(); err != nil {
		return nil, nil, err
	}
	Setup(nil)
	remember(t.Secrets()...)
	if t.Provider == S3 {
		if err := p.precheck(ctx, t.Endpoint); err != nil {
			return nil, nil, err
		}
	}
	ctx, err := egressContext(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	name, opts := t.backend()
	info, err := fs.Find(name)
	if err != nil {
		return nil, nil, err
	}
	// no remote name: options + backend defaults only, nothing from a config file or per-remote variables
	f, err := info.NewFs(ctx, "", t.root(), fs.ConfigMap(info.Prefix, info.Options, "", opts))
	if err != nil && !errors.Is(err, fs.ErrorIsFile) {
		return nil, nil, errors.New(Clean(err.Error()))
	}
	return f, ctx, nil
}
