//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// objectStorage runs after a recorded SSH session (`./deploy/e2e.sh storage`): upload to an S3 bucket with versioning,
// local copy removed through the policy, playback from the bucket, then the retention purge deletes every version.
//
//	GG_E2E_S3        "<endpoint seen by the app> <bucket> <access key> <secret key>"
//	GG_E2E_S3_HOST   the same endpoint seen from this host (lists object versions)
//	GG_E2E_DB_EXEC   command that runs one SQL statement (appended) in the app's database
//	GG_E2E_REC_EXEC  command that runs one shell command (appended) where guacd writes the recordings
func objectStorage(t *testing.T, admin *client, q string, hist map[string]any) {
	f := strings.Fields(os.Getenv("GG_E2E_S3"))
	if len(f) != 4 {
		t.Fatal("GG_E2E_S3 needs endpoint, bucket, access key and secret key")
	}
	hid, tunnel := hist["id"].(string), hist["tunnelId"].(string)
	target := map[string]any{"provider": "s3", "endpoint": f[0], "bucket": f[1], "prefix": "e2e", "pathStyle": true,
		"accessKey": f[2], "secretKey": f[3]}

	st, out := admin.do("POST", "/api/v1/settings/recording/storage/test", map[string]any{"target": target})
	if st != 200 || out["ok"] != true || out["versioned"] != true {
		t.Fatalf("storage test: %d %v", st, out)
	}
	st, out = admin.do("PUT", "/api/v1/settings/recording/storage", map[string]any{"enabled": true, "keepLocal": false, "target": target})
	must(t, st, 200, out, "storage settings")

	// guacd may still be flushing the screen recording after the session ended: wait until its size settles, or the
	// upload sees the file change and backs off longer than this test waits
	prev := ""
	for i := 0; i < 30; i++ {
		cur := run(t, "GG_E2E_REC_EXEC", "stat -c %s /record/"+tunnel)
		if cur == prev {
			break
		}
		prev = cur
		time.Sleep(2 * time.Second)
	}
	// the session ended two days ago: past the quiet period, the local removal window and a 1-day retention
	shift := int64(48 * time.Hour / time.Millisecond)
	run(t, "GG_E2E_DB_EXEC", fmt.Sprintf("UPDATE connection_history SET start_at=start_at-%d, end_at=end_at-%d WHERE id=%s", shift, shift, hid))
	run(t, "GG_E2E_REC_EXEC", fmt.Sprintf("touch -d @%d /record/%s*", time.Now().Add(-48*time.Hour).Unix(), tunnel))

	st, out = admin.do("POST", "/api/v1/settings/recording/storage/backfill", nil)
	must(t, st, 200, out, "upload existing recordings")
	var h map[string]any
	for i := 0; i < 120; i++ { // an upload retried by asynq waits 15–45 s
		h = historyRow(t, admin, q, hid)
		if h["videoStorage"] == "remote" && h["textStorage"] == "remote" && h["videoLocalRemoved"] == true && h["textLocalRemoved"] == true {
			break
		}
		time.Sleep(time.Second)
	}
	if h["videoLocalRemoved"] != true || h["textLocalRemoved"] != true {
		t.Fatalf("not uploaded and removed locally: %v", h)
	}
	if out := run(t, "GG_E2E_REC_EXEC", "ls /record/ | grep -c "+tunnel+" || true"); strings.TrimSpace(out) != "0" {
		t.Fatalf("local files left: %s", out)
	}

	// playback and transcript now come from the bucket
	if code, body := admin.raw("GET", "/api/v1/history/"+hid+"/typescript"); code != 200 || !strings.Contains(body, "echo GGREC-OK") {
		t.Fatalf("typescript from storage: %d %q", code, body)
	}
	if code, body := admin.raw("GET", "/api/v1/history/"+hid+"/recording"); code != 200 || !strings.Contains(body, ".size,") {
		t.Fatalf("screen recording from storage: %d len=%d", code, len(body))
	}

	// retention of one day (shortening asks for confirmation), then purge now
	st, out = admin.do("GET", "/api/v1/settings/recording", nil)
	must(t, st, 200, out, "recording settings")
	d := out["defaults"].(map[string]any)
	d["retentionVideoDays"], d["retentionTextDays"] = 1, 1
	if st, out = admin.do("PUT", "/api/v1/settings/recording", d); st == 409 {
		d["previewToken"] = out["data"].(map[string]any)["token"]
		st, out = admin.do("PUT", "/api/v1/settings/recording", d)
	}
	must(t, st, 200, out, "retention 1 day")
	st, out = admin.do("POST", "/api/v1/recordings/purge", nil)
	if st != 200 || out["remote"].(float64) < 3 {
		t.Fatalf("purge: %d %v", st, out)
	}
	if h = historyRow(t, admin, q, hid); h["videoStorage"] != "deleted" || h["textStorage"] != "deleted" {
		t.Fatalf("after purge: %v", h)
	}
	// no version and no delete marker of the session's objects left in the bucket
	host := os.Getenv("GG_E2E_S3_HOST")
	list, err := exec.Command("curl", "-fsS", "--aws-sigv4", "aws:amz:us-east-1:s3", "-u", f[2]+":"+f[3],
		host+"/"+f[1]+"?versions&prefix=e2e/").CombinedOutput()
	if err != nil {
		t.Fatalf("list versions: %v %s", err, list)
	}
	if strings.Contains(string(list), "/"+hid+"/") {
		t.Fatalf("object versions left after the purge: %s", list)
	}
}

// historyRow finds the session by id (moving its start back changes its place in the list).
func historyRow(t *testing.T, admin *client, q, id string) map[string]any {
	st, out := admin.do("GET", "/api/v1/history?q="+q+"&limit=100", nil)
	must(t, st, 200, out, "history")
	for _, it := range out["items"].([]any) {
		if h := it.(map[string]any); h["id"] == id {
			return h
		}
	}
	t.Fatalf("history row %s not found", id)
	return nil
}

// run executes the command in env var k with arg appended as its last argument.
func run(t *testing.T, k, arg string) string {
	cmd := strings.Fields(os.Getenv(k))
	if len(cmd) == 0 {
		t.Fatalf("%s not set", k)
	}
	b, err := exec.Command(cmd[0], append(cmd[1:], arg)...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %q: %v %s", k, arg, err, b)
	}
	return string(b)
}
