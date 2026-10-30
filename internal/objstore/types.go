// Package objstore copies recording sets to object storage (AWS S3, S3-compatible services incl. OCI's S3 API,
// OCI Object Storage with instance/workload principal, Google Cloud Storage) through rclone used as a library:
// only these backends are linked, configuration lives in memory (never rclone.conf), traffic goes through the
// endpoint-checking egress proxy and rclone's logs are sanitized (openspec add-async-jobs-object-storage).
package objstore

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Available reports whether object storage is compiled in (false with the noobjstore build tag).
const Available = available

// ErrUnavailable is returned by Connect when the binary was built without object storage.
var ErrUnavailable = errors.New("object storage is not available in this build")

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

	withVersions bool // S3 family: list every version and delete marker as an object (internal)
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

func (t Target) root() string {
	p := strings.Trim(t.Prefix, "/")
	if p == "" {
		return t.Bucket
	}
	return t.Bucket + "/" + p
}

// Stored describes a verified object.
type Stored struct {
	Key    string `json:"key"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// ErrMismatch: the object read back from the provider differs from the local file.
var ErrMismatch = errors.New("object in storage differs from the local file")

// Entry is a current object for the orphan sweep.
type Entry struct {
	Key     string
	ModTime time.Time
}

// Policy says which destinations object storage may reach.
type Policy struct {
	AllowHTTP bool     // plain HTTP endpoints (operator configuration)
	Allowed   []string // hosts or CIDRs allowed even when private (operator configuration)
}

// ErrEndpointNotAllowed is returned for destinations refused by the policy.
var ErrEndpointNotAllowed = errors.New("object storage endpoint not allowed")

// ProbeResult is what a connection test learned about the bucket.
type ProbeResult struct {
	Versioned     bool `json:"versioned"`     // deleted objects leave versions or markers behind
	DeleteRefused bool `json:"deleteRefused"` // object lock, retention or permissions refuse deletion
}
