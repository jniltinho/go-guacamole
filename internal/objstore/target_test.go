//go:build !noobjstore

package objstore

import "testing"

func TestOCIConfiguration(t *testing.T) {
	compat := Target{Provider: OCI, Namespace: "axyz", Region: "sa-saopaulo-1", Bucket: "rec", AccessKey: "k", SecretKey: "s"}
	if err := compat.Validate(); err != nil {
		t.Fatal(err)
	}
	name, m := compat.backend()
	if name != "s3" || m["endpoint"] != "https://axyz.compat.objectstorage.sa-saopaulo-1.oraclecloud.com" ||
		m["force_path_style"] != "true" || m["provider"] != "Other" {
		t.Fatalf("OCI S3 compatibility: %s %v", name, m)
	}
	native := Target{Provider: OCINative, Namespace: "axyz", Region: "sa-saopaulo-1", Compartment: "ocid1.compartment.oc1..x",
		Bucket: "rec", OCIAuth: "instance_principal_auth"}
	if err := native.Validate(); err != nil {
		t.Fatal(err)
	}
	if name, m := native.backend(); name != "oracleobjectstorage" || m["provider"] != "instance_principal_auth" || m["compartment"] == "" {
		t.Fatalf("OCI native: %s %v", name, m)
	}
	native.OCIAuth = "user_principal_auth" // needs an OCI config file on disk: not supported
	if native.Validate() == nil {
		t.Fatal("user principal accepted")
	}
}
