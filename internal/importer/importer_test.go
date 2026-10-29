package importer_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/jniltinho/go-guacamole/internal/access"
	"github.com/jniltinho/go-guacamole/internal/auth"
	"github.com/jniltinho/go-guacamole/internal/importer"
	"github.com/jniltinho/go-guacamole/internal/store"
	"github.com/jniltinho/go-guacamole/internal/store/storetest"
)

// fixtures on top of the official schema (which seeds guacadmin with ADMINISTER).
const fixtures = `
INSERT INTO guacamole_entity (name, type) VALUES ('alice','USER'),('bob','USER'),('carol','USER'),
  ('ops','USER_GROUP'),('sub-ops','USER_GROUP');
INSERT INTO guacamole_user (entity_id, password_hash, password_salt, password_date, expired, access_window_start, access_window_end, timezone)
  SELECT entity_id, UNHEX(SHA2(CONCAT('alice-pass', HEX(UNHEX(SHA2('salt', 256)))), 256)), UNHEX(SHA2('salt', 256)), NOW(), 1, '08:00:00', '18:00:00', 'America/Sao_Paulo'
  FROM guacamole_entity WHERE name='alice';
INSERT INTO guacamole_user (entity_id, password_hash, password_date) SELECT entity_id, UNHEX(SHA2('bob-pass',256)), NOW() FROM guacamole_entity WHERE name='bob';
INSERT INTO guacamole_user (entity_id, password_hash, password_date) SELECT entity_id, UNHEX(SHA2('carol-pass',256)), NOW() FROM guacamole_entity WHERE name='carol';
INSERT INTO guacamole_user_group (entity_id, disabled) SELECT entity_id, 0 FROM guacamole_entity WHERE name='ops';
INSERT INTO guacamole_user_group (entity_id, disabled) SELECT entity_id, 1 FROM guacamole_entity WHERE name='sub-ops';
-- alice ∈ ops ; carol ∈ sub-ops (disabled) ∈ ops
INSERT INTO guacamole_user_group_member (user_group_id, member_entity_id)
  SELECT g.user_group_id, e.entity_id FROM guacamole_user_group g JOIN guacamole_entity ge ON ge.entity_id=g.entity_id, guacamole_entity e
  WHERE ge.name='ops' AND e.name IN ('alice','sub-ops');
INSERT INTO guacamole_user_group_member (user_group_id, member_entity_id)
  SELECT g.user_group_id, e.entity_id FROM guacamole_user_group g JOIN guacamole_entity ge ON ge.entity_id=g.entity_id, guacamole_entity e
  WHERE ge.name='sub-ops' AND e.name='carol';
INSERT INTO guacamole_connection_group (connection_group_name, type) VALUES ('Servers','ORGANIZATIONAL'),('Pool','BALANCING');
INSERT INTO guacamole_connection (connection_name, parent_id, protocol)
  SELECT 'web', connection_group_id, 'ssh' FROM guacamole_connection_group WHERE connection_group_name='Servers';
INSERT INTO guacamole_connection (connection_name, parent_id, protocol)
  SELECT 'db', connection_group_id, 'ssh' FROM guacamole_connection_group WHERE connection_group_name='Servers';
INSERT INTO guacamole_connection (connection_name, parent_id, protocol)
  SELECT 'pool-1', connection_group_id, 'rdp' FROM guacamole_connection_group WHERE connection_group_name='Pool';
INSERT INTO guacamole_connection_parameter VALUES
  ((SELECT connection_id FROM guacamole_connection WHERE connection_name='web'), 'hostname', '10.0.0.1'),
  ((SELECT connection_id FROM guacamole_connection WHERE connection_name='web'), 'password', 'S3cret!');
-- ops: READ on web and on folder Servers (folder READ must not expand to db)
INSERT INTO guacamole_connection_permission (entity_id, connection_id, permission)
  SELECT e.entity_id, c.connection_id, 'READ' FROM guacamole_entity e, guacamole_connection c WHERE e.name='ops' AND c.connection_name='web';
INSERT INTO guacamole_connection_group_permission (entity_id, connection_group_id, permission)
  SELECT e.entity_id, g.connection_group_id, 'READ' FROM guacamole_entity e, guacamole_connection_group g WHERE e.name='ops' AND g.connection_group_name='Servers';
-- bob: READ on the balancing group only
INSERT INTO guacamole_connection_group_permission (entity_id, connection_group_id, permission)
  SELECT e.entity_id, g.connection_group_id, 'READ' FROM guacamole_entity e, guacamole_connection_group g WHERE e.name='bob' AND g.connection_group_name='Pool';
`

func sourceDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN not set")
	}
	root, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, q := range []string{"DROP DATABASE IF EXISTS guac_src", "CREATE DATABASE guac_src"} {
		if _, err := root.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	base := dsn[:strings.LastIndex(dsn, "/")+1]
	src, err := sql.Open("mysql", base+"guac_src?multiStatements=true")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { src.Close() })
	schema, err := os.ReadFile(filepath.Join("testdata", "guacamole-1.6-mysql.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := src.Exec(string(schema)); err != nil {
		t.Fatalf("load schema: %v", err)
	}
	if _, err := src.Exec(fixtures); err != nil {
		t.Fatalf("load fixtures: %v", err)
	}
	return src
}

func checksum(t *testing.T, db *sql.DB) string {
	var parts []string
	for _, tbl := range []string{"guacamole_entity", "guacamole_user", "guacamole_connection", "guacamole_connection_permission"} {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + tbl).Scan(&n); err != nil {
			t.Fatal(err)
		}
		parts = append(parts, tbl, string(rune('0'+n%10)))
	}
	return strings.Join(parts, ",")
}

func TestImportFromGuacamole(t *testing.T) {
	src := sourceDB(t)
	dst := storetest.Open(t, "sqlite", filepath.Join(t.TempDir(), "dst.db"))
	ctx := context.Background()
	before := checksum(t, src)

	rep, err := importer.Run(ctx, src, dst, importer.Options{SourceDriver: "mysql", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := dst.CountUsers(ctx); n != 0 || rep.Created["users"] != 4 {
		t.Fatalf("dry run wrote data or wrong counts: users=%d report=%+v", n, rep.Created)
	}

	rep, err = importer.Run(ctx, src, dst, importer.Options{SourceDriver: "mysql"})
	if err != nil {
		t.Fatal(err)
	}
	if checksum(t, src) != before {
		t.Fatal("source database was modified")
	}
	joined := strings.Join(rep.Warnings, "\n")
	for _, w := range []string{"balancing group", "connection-group READ", "--accept-expansion", "nested group"} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing warning %q in:\n%s", w, joined)
		}
	}
	if len(rep.Diffs) != 0 {
		t.Fatalf("unexpected access differences: %v", rep.Diffs)
	}

	// stock guacadmin keeps its password and admin rights
	ga, err := dst.GetUserByName(ctx, "guacadmin")
	if err != nil || !auth.Verify(ga, "guacadmin").OK {
		t.Fatalf("guacadmin login: %v", err)
	}
	if e, _ := access.Resolve(ctx, dst, ga.ID); !e.IsAdmin() {
		t.Fatal("guacadmin lost ADMINISTER")
	}
	// alice: salted legacy hash, expired → must change, window and timezone preserved, READ on web only
	alice, _ := dst.GetUserByName(ctx, "alice")
	if !auth.Verify(alice, "alice-pass").OK || !alice.MustChangePassword || alice.AccessWindowEnd == nil || *alice.AccessWindowEnd != "18:00:00" ||
		alice.Timezone == nil || *alice.Timezone != "America/Sao_Paulo" {
		t.Fatalf("alice restrictions: %+v", alice)
	}
	conns, _ := dst.ListConnections(ctx)
	byName := map[string]*store.Connection{}
	for _, c := range conns {
		byName[c.Name] = c
	}
	ea, _ := access.Resolve(ctx, dst, alice.ID)
	if !ea.CanRead(byName["web"]) || ea.CanRead(byName["db"]) {
		t.Fatal("folder READ must not expand access to db")
	}
	// carol is only reachable through a disabled group → no access
	carol, _ := dst.GetUserByName(ctx, "carol")
	if ec, _ := access.Resolve(ctx, dst, carol.ID); ec.CanRead(byName["web"]) {
		t.Fatal("access through a disabled nested group")
	}
	// bob: balancing READ skipped without --accept-expansion
	bob, _ := dst.GetUserByName(ctx, "bob")
	if eb, _ := access.Resolve(ctx, dst, bob.ID); eb.CanRead(byName["pool-1"]) {
		t.Fatal("balancing READ imported without acceptance")
	}
	// secrets encrypted and readable
	web, _ := dst.GetConnection(ctx, byName["web"].ID)
	if web.Parameters["password"] != "S3cret!" {
		t.Fatal("secret parameter not imported")
	}

	// idempotent re-run, then accept expansion
	rep, err = importer.Run(ctx, src, dst, importer.Options{SourceDriver: "mysql", AcceptExpansion: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Created["users"] != 0 || rep.Skipped["users"] != 4 || rep.Created["connections"] != 0 {
		t.Fatalf("re-run not idempotent: %+v / %+v", rep.Created, rep.Skipped)
	}
	if eb, _ := access.Resolve(ctx, dst, bob.ID); !eb.CanRead(byName["pool-1"]) {
		t.Fatal("--accept-expansion did not grant pool children")
	}
}

func TestImportReportsPrivilegeDifferences(t *testing.T) {
	src := sourceDB(t)
	dst := storetest.Open(t, "sqlite", filepath.Join(t.TempDir(), "dst.db"))
	ctx := context.Background()
	// an existing target account with the same name as a non-admin source user
	bob := &store.User{Username: "bob", PasswordHash: "x"}
	if err := dst.CreateUser(ctx, bob); err != nil {
		t.Fatal(err)
	}
	_ = dst.GrantSystem(ctx, store.SubjectUser, bob.ID, store.PermAdminister)
	rep, err := importer.Run(ctx, src, dst, importer.Options{SourceDriver: "mysql", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range rep.Diffs {
		if strings.HasPrefix(d, "bob: GAINED ADMINISTER") {
			found = true
		}
	}
	if !found {
		t.Fatalf("privilege expansion not reported: %v", rep.Diffs)
	}
}

const pgFixtures = `
INSERT INTO guacamole_entity (name, type) VALUES ('alice','USER'),('ops','USER_GROUP');
INSERT INTO guacamole_user (entity_id, password_hash, password_salt, password_date, expired, access_window_start, access_window_end, timezone)
  SELECT entity_id, sha256(convert_to('alice-pass' || upper(encode(sha256('salt'::bytea), 'hex')), 'UTF8')), sha256('salt'::bytea),
         now(), true, '08:00:00', '18:00:00', 'America/Sao_Paulo' FROM guacamole_entity WHERE name='alice';
INSERT INTO guacamole_user_group (entity_id, disabled) SELECT entity_id, false FROM guacamole_entity WHERE name='ops';
INSERT INTO guacamole_user_group_member (user_group_id, member_entity_id)
  SELECT g.user_group_id, e.entity_id FROM guacamole_user_group g JOIN guacamole_entity ge ON ge.entity_id=g.entity_id, guacamole_entity e
  WHERE ge.name='ops' AND e.name='alice';
INSERT INTO guacamole_connection_group (connection_group_name, type) VALUES ('Servers','ORGANIZATIONAL');
INSERT INTO guacamole_connection (connection_name, parent_id, protocol)
  SELECT n, connection_group_id, 'ssh' FROM guacamole_connection_group, (VALUES ('web'),('db')) v(n) WHERE connection_group_name='Servers';
INSERT INTO guacamole_connection_parameter
  SELECT connection_id, 'password', 'S3cret!' FROM guacamole_connection WHERE connection_name='web';
INSERT INTO guacamole_connection_permission (entity_id, connection_id, permission)
  SELECT e.entity_id, c.connection_id, 'READ' FROM guacamole_entity e, guacamole_connection c WHERE e.name='ops' AND c.connection_name='web';
INSERT INTO guacamole_connection_group_permission (entity_id, connection_group_id, permission)
  SELECT e.entity_id, g.connection_group_id, 'READ' FROM guacamole_entity e, guacamole_connection_group g WHERE e.name='ops';
`

func TestImportFromGuacamolePostgres(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	root, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, q := range []string{"DROP DATABASE IF EXISTS guac_src", "CREATE DATABASE guac_src"} {
		if _, err := root.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	srcDSN := strings.Replace(dsn, "/gg?", "/guac_src?", 1)
	src, err := sql.Open("pgx", srcDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	schema, err := os.ReadFile(filepath.Join("testdata", "guacamole-1.6-postgres.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, chunk := range []string{string(schema), pgFixtures} {
		if _, err := src.Exec(chunk); err != nil {
			t.Fatalf("load: %v", err)
		}
	}
	dst := storetest.Open(t, "sqlite", filepath.Join(t.TempDir(), "dst.db"))
	ctx := context.Background()
	rep, err := importer.Run(ctx, src, dst, importer.Options{SourceDriver: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Diffs) != 0 || rep.Created["users"] != 2 || rep.Created["connections"] != 2 {
		t.Fatalf("report: %+v %v", rep.Created, rep.Diffs)
	}
	ga, _ := dst.GetUserByName(ctx, "guacadmin")
	alice, _ := dst.GetUserByName(ctx, "alice")
	if !auth.Verify(ga, "guacadmin").OK || !auth.Verify(alice, "alice-pass").OK || !alice.MustChangePassword ||
		alice.AccessWindowStart == nil || *alice.AccessWindowStart != "08:00:00" {
		t.Fatalf("users not imported correctly: %+v", alice)
	}
	conns, _ := dst.ListConnections(ctx)
	ea, _ := access.Resolve(ctx, dst, alice.ID)
	for _, c := range conns {
		if want := c.Name == "web"; ea.CanRead(c) != want {
			t.Errorf("alice READ on %s = %v, want %v", c.Name, ea.CanRead(c), want)
		}
	}
}
