package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jniltinho/go-guacamole/internal/secrets"
)

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store implements all repositories over a DB or a transaction.
type Store struct {
	db  *DB
	q   querier
	box *secrets.Box
	now func() time.Time
}

// New returns a Store over db. box encrypts secret connection parameters.
func New(db *DB, box *secrets.Box) *Store {
	return &Store{db: db, q: db.DB, box: box, now: time.Now}
}

// DB returns the underlying database.
func (s *Store) DB() *DB { return s.db }

// Box returns the secrets box.
func (s *Store) Box() *secrets.Box { return s.box }

// Now returns the current time in Unix milliseconds.
func (s *Store) Now() int64 { return s.now().UnixMilli() }

// Tx runs fn inside a transaction.
func (s *Store) Tx(ctx context.Context, fn func(tx *Store) error) error {
	if _, ok := s.q.(*sql.Tx); ok {
		return fn(s)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	child := *s
	child.q = tx
	if err := fn(&child); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	res, err := s.q.ExecContext(ctx, s.db.D.Rebind(query), args...)
	return res, mapErr(err)
}

func (s *Store) query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.q.QueryContext(ctx, s.db.D.Rebind(query), args...)
}

func (s *Store) row(ctx context.Context, query string, args ...any) *sql.Row {
	return s.q.QueryRowContext(ctx, s.db.D.Rebind(query), args...)
}

// insert executes an INSERT and returns the generated id.
func (s *Store) insert(ctx context.Context, query string, args ...any) (int64, error) {
	if s.db.D.Name == "postgres" {
		var id int64
		err := s.row(ctx, query+" RETURNING id", args...).Scan(&id)
		return id, mapErr(err)
	}
	res, err := s.exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate") {
		return ErrConflict
	}
	return err
}

func affected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RandomHex returns n random bytes hex-encoded.
func RandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func nullStr(p *string) any {
	if p == nil || *p == "" {
		return nil
	}
	return *p
}

func nullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullInt64(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func strPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

func intPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func int64Ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func int64Args(ids []int64) []any {
	out := make([]any, len(ids))
	for i, v := range ids {
		out[i] = v
	}
	return out
}

// AccessEpoch returns the global access epoch counter.
func (s *Store) AccessEpoch(ctx context.Context) (string, error) {
	var v string
	err := s.row(ctx, `SELECT value FROM system_state WHERE name = 'access_epoch'`).Scan(&v)
	return v, mapErr(err)
}

// BumpAccessEpoch signals other processes that grants or the tree changed.
func (s *Store) BumpAccessEpoch(ctx context.Context) error {
	_, err := s.exec(ctx, `UPDATE system_state SET value = ? WHERE name = 'access_epoch'`, RandomHex(8))
	return err
}

// Ping checks database connectivity.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
