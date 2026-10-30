// Package store is the persistence layer. One SQL implementation serves
// SQLite, MySQL/MariaDB and PostgreSQL through a small dialect layer.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Dialect captures the differences between supported databases.
type Dialect struct {
	Name   string
	driver string
	tokens map[string]string
}

var dialects = map[string]Dialect{
	"sqlite": {Name: "sqlite", driver: "sqlite", tokens: map[string]string{
		"{{ID}}": "INTEGER PRIMARY KEY AUTOINCREMENT", "{{BOOL}}": "INTEGER", "{{FALSE}}": "0",
		"{{TS}}": "BIGINT", "{{STR}}": "VARCHAR", "{{TEXT}}": "TEXT", "{{ENGINE}}": "",
	}},
	"mysql": {Name: "mysql", driver: "mysql", tokens: map[string]string{
		"{{ID}}": "BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY", "{{BOOL}}": "BOOLEAN", "{{FALSE}}": "0",
		"{{TS}}": "BIGINT", "{{STR}}": "VARCHAR", "{{TEXT}}": "MEDIUMTEXT",
		"{{ENGINE}}": " ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin",
	}},
	"postgres": {Name: "postgres", driver: "pgx", tokens: map[string]string{
		"{{ID}}": "BIGSERIAL PRIMARY KEY", "{{BOOL}}": "BOOLEAN", "{{FALSE}}": "FALSE",
		"{{TS}}": "BIGINT", "{{STR}}": "VARCHAR", "{{TEXT}}": "TEXT", "{{ENGINE}}": "",
	}},
}

// CastInt wraps an expression so aggregates scan as int64 on every driver.
func (d Dialect) CastInt(expr string) string {
	switch d.Name {
	case "mysql":
		return "CAST(" + expr + " AS SIGNED)"
	case "postgres":
		return "CAST(" + expr + " AS BIGINT)"
	}
	return "CAST(" + expr + " AS INTEGER)"
}

// Rebind converts '?' placeholders to the dialect's syntax.
func (d Dialect) Rebind(q string) string {
	if d.Name != "postgres" {
		return q
	}
	var b strings.Builder
	n := 0
	inStr := false
	for i := 0; i < len(q); i++ {
		c := q[i]
		if c == '\'' {
			inStr = !inStr
		}
		if c == '?' && !inStr {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// DB wraps *sql.DB with the active dialect.
type DB struct {
	*sql.DB
	D Dialect
}

// Open connects to the database for the given driver name and DSN.
func Open(ctx context.Context, driver, dsn string, maxOpen int) (*DB, error) {
	d, ok := dialects[driver]
	if !ok {
		return nil, fmt.Errorf("unsupported DB_DRIVER %q (supported: sqlite, mysql, postgres)", driver)
	}
	switch driver {
	case "sqlite":
		if dir := filepath.Dir(dsn); dsn != ":memory:" && !strings.HasPrefix(dsn, "file:") {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return nil, fmt.Errorf("create sqlite dir: %w", err)
			}
		}
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn += sep + "_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_txlock=immediate"
		maxOpen = 1
	}
	sqlDB, err := sql.Open(d.driver, dsn)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("connect %s: %w", driver, err)
	}
	return &DB{DB: sqlDB, D: d}, nil
}

// Migration is one versioned schema step.
type Migration struct {
	Version int
	Name    string
	Up      string
	Down    string
}

func (db *DB) migrations() ([]Migration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	byVer := map[int]*Migration{}
	for _, e := range entries {
		name := e.Name()
		parts := strings.SplitN(name, "_", 2)
		v, err := strconv.Atoi(parts[0])
		if err != nil || len(parts) < 2 {
			return nil, fmt.Errorf("bad migration name %s", name)
		}
		raw, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		sqlText := string(raw)
		for k, val := range db.D.tokens {
			sqlText = strings.ReplaceAll(sqlText, k, val)
		}
		m := byVer[v]
		if m == nil {
			m = &Migration{Version: v}
			byVer[v] = m
		}
		switch {
		case strings.HasSuffix(name, ".up.sql"):
			m.Up, m.Name = sqlText, strings.TrimSuffix(parts[1], ".up.sql")
		case strings.HasSuffix(name, ".down.sql"):
			m.Down = sqlText
		}
	}
	out := make([]Migration, 0, len(byVer))
	for _, m := range byVer {
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

func (db *DB) ensureMigrationTable(ctx context.Context) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT NOT NULL PRIMARY KEY, applied_at BIGINT NOT NULL)`)
	return err
}

func (db *DB) applied(ctx context.Context) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// MigrationStatus describes one migration and whether it is applied.
type MigrationStatus struct {
	Version int
	Name    string
	Applied bool
}

// Status lists migrations and their state.
func (db *DB) Status(ctx context.Context) ([]MigrationStatus, error) {
	if err := db.ensureMigrationTable(ctx); err != nil {
		return nil, err
	}
	ms, err := db.migrations()
	if err != nil {
		return nil, err
	}
	ap, err := db.applied(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MigrationStatus, 0, len(ms))
	for _, m := range ms {
		out = append(out, MigrationStatus{m.Version, m.Name, ap[m.Version]})
	}
	return out, nil
}

// MigrateUp applies all pending migrations and returns how many ran.
func (db *DB) MigrateUp(ctx context.Context) (int, error) {
	if err := db.ensureMigrationTable(ctx); err != nil {
		return 0, err
	}
	ms, err := db.migrations()
	if err != nil {
		return 0, err
	}
	ap, err := db.applied(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, m := range ms {
		if ap[m.Version] {
			continue
		}
		if err := db.runScript(ctx, m.Up, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, m.Version, time.Now().UnixMilli()); err != nil {
			return n, fmt.Errorf("migration %04d_%s: %w", m.Version, m.Name, err)
		}
		n++
	}
	return n, nil
}

// MigrateDown rolls back the most recent applied migration.
func (db *DB) MigrateDown(ctx context.Context) (*Migration, error) {
	if err := db.ensureMigrationTable(ctx); err != nil {
		return nil, err
	}
	ms, err := db.migrations()
	if err != nil {
		return nil, err
	}
	ap, err := db.applied(ctx)
	if err != nil {
		return nil, err
	}
	for i := len(ms) - 1; i >= 0; i-- {
		m := ms[i]
		if !ap[m.Version] {
			continue
		}
		if m.Name == "recording_policy" {
			// older builds purge by file age: never drop legal holds silently
			var held int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM connection_history WHERE hold_at IS NOT NULL`).Scan(&held); err != nil {
				return nil, err
			}
			if held > 0 {
				return nil, fmt.Errorf("rollback %04d_%s refused: %d history entries are under legal hold; release them first", m.Version, m.Name, held)
			}
		}
		if err := db.runScript(ctx, m.Down, `DELETE FROM schema_migrations WHERE version = ?`, m.Version); err != nil {
			return nil, fmt.Errorf("rollback %04d_%s: %w", m.Version, m.Name, err)
		}
		return &m, nil
	}
	return nil, errors.New("no applied migrations")
}

func (db *DB) runScript(ctx context.Context, script, record string, args ...any) error {
	stmts := splitSQL(script)
	// MySQL DDL is not transactional; SQLite and PostgreSQL are.
	if db.D.Name == "mysql" {
		for _, s := range stmts {
			if _, err := db.ExecContext(ctx, s); err != nil {
				return err
			}
		}
		_, err := db.ExecContext(ctx, db.D.Rebind(record), args...)
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, db.D.Rebind(record), args...); err != nil {
		return err
	}
	return tx.Commit()
}

func splitSQL(script string) []string {
	var out []string
	var cur strings.Builder
	for _, line := range strings.Split(script, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "--") || trim == "" {
			continue
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
		if strings.HasSuffix(trim, ";") {
			s := strings.TrimSuffix(strings.TrimSpace(cur.String()), ";")
			out = append(out, s)
			cur.Reset()
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		out = append(out, s)
	}
	return out
}
