// Package storetest opens migrated databases for tests. SQLite always runs;
// MariaDB/MySQL and PostgreSQL run when TEST_MYSQL_DSN / TEST_POSTGRES_DSN are set.
package storetest

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jniltinho/go-guacamole/internal/secrets"
	"github.com/jniltinho/go-guacamole/internal/store"
)

// TestKey is the APP_SECRET_KEY used by tests.
const TestKey = "test-secret-key-0123456789abcdef0123"

// Drivers returns the driver → DSN pairs available for this test run.
func Drivers(t testing.TB) map[string]string {
	out := map[string]string{"sqlite": filepath.Join(t.TempDir(), "test.db")}
	if dsn := os.Getenv("TEST_MYSQL_DSN"); dsn != "" {
		out["mysql"] = dsn
	}
	if dsn := os.Getenv("TEST_POSTGRES_DSN"); dsn != "" {
		out["postgres"] = dsn
	}
	return out
}

// isolate creates a throwaway database on a server driver (MySQL/PostgreSQL) so
// packages running in parallel never share a schema, and drops it on cleanup.
func isolate(t testing.TB, driver, dsn string) string {
	t.Helper()
	name := "gg_t_" + store.RandomHex(6)
	sqlDriver := map[string]string{"mysql": "mysql", "postgres": "pgx"}[driver]
	admin, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		admin.Close()
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + name)
		admin.Close()
	})
	if driver == "postgres" {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		u.Path = "/" + name
		return u.String()
	}
	slash := strings.LastIndex(dsn, "/")
	rest := ""
	if q := strings.Index(dsn[slash:], "?"); q >= 0 {
		rest = dsn[slash+q:]
	}
	return dsn[:slash+1] + name + rest
}

// Open opens and migrates a database (a fresh isolated one for server drivers).
func Open(t testing.TB, driver, dsn string) *store.Store {
	t.Helper()
	ctx := context.Background()
	if driver != "sqlite" {
		dsn = isolate(t, driver, dsn)
	}
	db, err := store.Open(ctx, driver, dsn, 5)
	if err != nil {
		t.Fatalf("open %s: %v", driver, err)
	}
	if _, err := db.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate %s: %v", driver, err)
	}
	t.Cleanup(func() { db.Close() })
	box, err := secrets.NewBox(TestKey)
	if err != nil {
		t.Fatal(err)
	}
	return store.New(db, box)
}

// ForEach runs fn against every available driver as a subtest.
func ForEach(t *testing.T, fn func(t *testing.T, s *store.Store)) {
	for driver, dsn := range Drivers(t) {
		t.Run(driver, func(t *testing.T) { fn(t, Open(t, driver, dsn)) })
	}
}
