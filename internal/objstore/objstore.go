// Package objstore copies recording sets to object storage (AWS S3, S3-compatible services incl. OCI's S3 API,
// OCI Object Storage with instance/workload principal, Google Cloud Storage) through rclone used as a library:
// only these backends are linked, configuration lives in memory (never rclone.conf), traffic goes through the
// endpoint-checking egress proxy and rclone's logs are sanitized (openspec add-async-jobs-object-storage).
package objstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	_ "github.com/rclone/rclone/backend/googlecloudstorage"
	_ "github.com/rclone/rclone/backend/oracleobjectstorage"
	_ "github.com/rclone/rclone/backend/s3"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configmap"
)

// Providers.
const (
	AWS       = "aws"        // AWS S3
	S3        = "s3"         // any S3-compatible service (MinIO, Ceph, Wasabi, R2, B2 S3)
	OCI       = "oci"        // Oracle Cloud Object Storage through its S3 compatibility API (Customer Secret Keys)
	OCINative = "oci-native" // Oracle Cloud Object Storage native API, instance or workload principal (no secret)
	GCS       = "gcs"        // Google Cloud Storage, service account JSON
)

// Target is one object storage destination. Secret fields are sealed by the caller and never returned by the API.
type Target struct {
	Provider  string `json:"provider"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	Region    string `json:"region"`
	Endpoint  string `json:"endpoint"`  // S3-compatible
	PathStyle bool   `json:"pathStyle"` // S3-compatible
	Namespace string `json:"namespace"` // OCI
	// OCI native: instance_principal_auth or workload_identity_auth, and the bucket's compartment
	OCIAuth     string `json:"ociAuth"`
	Compartment string `json:"compartment"`
	SSE         string `json:"sse"`      // "" | "AES256" | "aws:kms"
	KMSKeyID    string `json:"kmsKeyId"` // with aws:kms
	// secrets
	AccessKey      string `json:"accessKey"`
	SecretKey      string `json:"secretKey,omitempty"`
	ServiceAccount string `json:"serviceAccount,omitempty"` // GCS JSON
}

// Secrets lists the secret values of t (for the sanitizer).
func (t Target) Secrets() []string { return []string{t.SecretKey, t.ServiceAccount} }

// Validate checks the fields needed by the provider.
func (t Target) Validate() error {
	if t.Bucket == "" || strings.ContainsAny(t.Bucket, "/ ") {
		return errors.New("bucket is required (no slashes or spaces)")
	}
	if strings.Contains(t.Prefix, "..") {
		return errors.New("prefix cannot contain ..")
	}
	switch t.Provider {
	case AWS:
		if t.Region == "" {
			return errors.New("region is required")
		}
	case S3:
		if t.Endpoint == "" {
			return errors.New("endpoint is required")
		}
	case OCI:
		if t.Namespace == "" || t.Region == "" {
			return errors.New("namespace and region are required")
		}
	case OCINative:
		if t.Namespace == "" || t.Region == "" || t.Compartment == "" {
			return errors.New("namespace, region and compartment are required")
		}
		if t.OCIAuth != "instance_principal_auth" && t.OCIAuth != "workload_identity_auth" {
			return errors.New("OCI native authentication must be instance or workload principal")
		}
	case GCS:
		if t.ServiceAccount == "" {
			return errors.New("service account JSON is required")
		}
	default:
		return fmt.Errorf("unknown provider %q", t.Provider)
	}
	if t.SSE != "" && t.SSE != "AES256" && t.SSE != "aws:kms" {
		return errors.New("server-side encryption must be AES256 or aws:kms")
	}
	return nil
}

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

func (t Target) root() string {
	p := strings.Trim(t.Prefix, "/")
	if p == "" {
		return t.Bucket
	}
	return t.Bucket + "/" + p
}

var (
	setupOnce sync.Once
	egressMu  sync.Mutex
	proxies   = map[string]*egress{} // per policy
	secretsMu sync.Mutex
	known     = NewSanitizer()
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

// remember adds secret values to the global sanitizer (rclone's logs, stored errors).
func remember(secrets ...string) {
	secretsMu.Lock()
	defer secretsMu.Unlock()
	known = NewSanitizer(append(known.values(), secrets...)...)
}

func (s *Sanitizer) values() []string {
	var out []string
	for i := 0; i < len(s.secrets); i += 3 { // value, std base64, url base64
		out = append(out, s.secrets[i])
	}
	return out
}

// Clean removes every known secret from text.
func Clean(text string) string {
	secretsMu.Lock()
	defer secretsMu.Unlock()
	return known.Clean(text)
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
	if t.Provider == S3 && strings.HasPrefix(strings.ToLower(t.Endpoint), "http://") && !p.AllowHTTP {
		return nil, nil, fmt.Errorf("%w: plain HTTP endpoint", ErrEndpointNotAllowed)
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
