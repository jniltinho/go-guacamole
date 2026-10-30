package store_test

import (
	"context"
	"testing"

	"github.com/jniltinho/go-guacamole/internal/store"
	"github.com/jniltinho/go-guacamole/internal/store/storetest"
)

func TestMigrateStatusAndDown(t *testing.T) {
	for driver, dsn := range storetest.Drivers(t) {
		t.Run(driver, func(t *testing.T) {
			s := storetest.Open(t, driver, dsn)
			ctx := context.Background()
			st, err := s.DB().Status(ctx)
			if err != nil || len(st) == 0 || !st[0].Applied {
				t.Fatalf("status: %+v %v", st, err)
			}
			if _, err := s.DB().MigrateDown(ctx); err != nil {
				t.Fatal(err)
			}
			if n, err := s.DB().MigrateUp(ctx); err != nil || n != 1 {
				t.Fatalf("re-up: %d %v", n, err)
			}
		})
	}
}

func TestRepositories(t *testing.T) {
	storetest.ForEach(t, func(t *testing.T, s *store.Store) {
		ctx := context.Background()
		u := &store.User{Username: "alice", PasswordHash: "x"}
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		if err := s.CreateUser(ctx, &store.User{Username: "alice", PasswordHash: "y"}); err != store.ErrConflict {
			t.Fatalf("duplicate user: %v", err)
		}
		got, err := s.GetUserByName(ctx, "alice")
		if err != nil || got.ID != u.ID || got.Disabled {
			t.Fatalf("get user: %+v %v", got, err)
		}

		g := &store.UserGroup{Name: "ops"}
		if err := s.CreateUserGroup(ctx, g); err != nil {
			t.Fatal(err)
		}
		if err := s.SetGroupMembers(ctx, g.ID, []int64{u.ID, u.ID}); err != nil {
			t.Fatal(err)
		}

		folder := &store.ConnectionGroup{Name: "Servers"}
		if err := s.CreateConnectionGroup(ctx, folder); err != nil {
			t.Fatal(err)
		}
		if err := s.CreateConnectionGroup(ctx, &store.ConnectionGroup{Name: "Servers"}); err != store.ErrConflict {
			t.Fatalf("duplicate folder: %v", err)
		}
		c := &store.Connection{Name: "srv", Protocol: "ssh", ParentID: &folder.ID,
			Parameters: map[string]string{"hostname": "10.0.0.1", "port": "22", "password": "Pa55"}}
		if err := s.CreateConnection(ctx, c); err != nil {
			t.Fatal(err)
		}
		var raw string
		if err := s.DB().QueryRowContext(ctx, s.DB().D.Rebind(`SELECT value FROM connection_parameters WHERE connection_id=? AND name='password'`), c.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if raw == "Pa55" {
			t.Fatal("password stored in plaintext")
		}
		full, err := s.GetConnection(ctx, c.ID)
		if err != nil || full.Parameters["password"] != "Pa55" {
			t.Fatalf("decrypt: %+v %v", full, err)
		}

		if err := s.SetGrants(ctx, store.SubjectGroup, g.ID, &store.Grants{Groups: []int64{folder.ID}, System: []string{store.PermAudit}}); err != nil {
			t.Fatal(err)
		}
		set, err := s.UserGrantSet(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(set.Groups[folder.ID]) != 1 || set.Groups[folder.ID][0].SourceName != "ops" || len(set.System[store.PermAudit]) != 1 {
			t.Fatalf("grant set: %+v", set)
		}
		g.Disabled = true
		if err := s.UpdateUserGroup(ctx, g); err != nil {
			t.Fatal(err)
		}
		set, _ = s.UserGrantSet(ctx, u.ID)
		if len(set.Groups) != 0 {
			t.Fatal("disabled group must grant nothing")
		}

		h := &store.HistoryEntry{UserID: &u.ID, Username: "alice", ConnectionID: &c.ID, ConnectionName: "srv", Protocol: "ssh", TunnelID: "t1", StartAt: s.Now()}
		if err := s.StartHistory(ctx, h); err != nil {
			t.Fatal(err)
		}
		if err := s.EndHistory(ctx, h.ID, s.Now(), "client_closed"); err != nil {
			t.Fatal(err)
		}
		if err := s.InsertAudit(ctx, &store.AuditEvent{ActorID: &u.ID, ActorName: "alice", Action: "connection.create", Outcome: "success",
			Metadata: map[string]any{"changed": []string{"parameters.password"}}}); err != nil {
			t.Fatal(err)
		}

		// deleting the connection keeps history with a NULL reference
		if err := s.DeleteConnection(ctx, c.ID); err != nil {
			t.Fatal(err)
		}
		hist, total, err := s.ListHistory(ctx, store.HistoryFilter{}, store.Page{})
		if err != nil || total != 1 || hist[0].ConnectionID != nil || hist[0].ConnectionName != "srv" || hist[0].EndAt == nil {
			t.Fatalf("history after delete: %+v %v", hist, err)
		}
		if err := s.DeleteUser(ctx, u.ID); err != nil {
			t.Fatal(err)
		}
		ev, _, err := s.ListAudit(ctx, store.AuditFilter{}, store.Page{})
		if err != nil || len(ev) != 1 || ev[0].ActorID != nil || ev[0].ActorName != "alice" {
			t.Fatalf("audit after delete: %+v %v", ev, err)
		}

		p := &store.Preferences{Theme: "win98", Locale: "pt-BR"}
		u2 := &store.User{Username: "bob", PasswordHash: "x"}
		_ = s.CreateUser(ctx, u2)
		if err := s.PutPreferences(ctx, u2.ID, p); err != nil {
			t.Fatal(err)
		}
		if err := s.PutPreferences(ctx, u2.ID, p); err != nil {
			t.Fatal(err)
		}
		gp, _ := s.GetPreferences(ctx, u2.ID)
		if gp.Theme != "win98" || gp.Locale != "pt-BR" {
			t.Fatalf("prefs: %+v", gp)
		}
		e1, _ := s.AccessEpoch(ctx)
		_ = s.BumpAccessEpoch(ctx)
		if e2, _ := s.AccessEpoch(ctx); e1 == e2 {
			t.Fatal("epoch not bumped")
		}
	})
}

func TestAccessStats(t *testing.T) {
	storetest.ForEach(t, func(t *testing.T, s *store.Store) {
		ctx := context.Background()
		add := func(user, conn string, start int64, end *int64) {
			h := &store.HistoryEntry{Username: user, ConnectionName: conn, Protocol: "ssh", TunnelID: user + conn, StartAt: start}
			if err := s.StartHistory(ctx, h); err != nil {
				t.Fatal(err)
			}
			if end != nil {
				_ = s.EndHistory(ctx, h.ID, *end, "client_closed")
			}
		}
		e1, e2 := int64(1_600_000), int64(1_300_000)
		add("alice", "web", 1_000_000, &e1) // 600 s
		add("bob", "web", 1_000_000, &e2)   // 300 s
		add("alice", "db", 1_000_000, nil)  // open: counts until now
		srv, err := s.AccessStats(ctx, "connection", 0, 1_100_000, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(srv) != 2 || srv[0].Key != "web" || srv[0].TotalMs != 900_000 || srv[0].Sessions != 2 || srv[0].Distinct != 2 ||
			srv[1].Key != "db" || srv[1].TotalMs != 100_000 || srv[1].ActiveCount != 1 {
			t.Fatalf("server stats: %+v", srv)
		}
		us, _ := s.AccessStats(ctx, "user", 0, 1_100_000, 10)
		if len(us) != 2 || us[0].Key != "alice" || us[0].TotalMs != 700_000 || us[0].Distinct != 2 {
			t.Fatalf("user stats: %+v", us)
		}
	})
}

func TestRecordingMigrationDownRefusedWithHold(t *testing.T) {
	storetest.ForEach(t, func(t *testing.T, s *store.Store) {
		ctx := context.Background()
		h := &store.HistoryEntry{Username: "u", ConnectionName: "c", Protocol: "ssh", TunnelID: "t1", StartAt: 1}
		if err := s.StartHistory(ctx, h); err != nil {
			t.Fatal(err)
		}
		if ok, err := s.SetHold(ctx, h.ID, "incident", "auditor"); !ok || err != nil {
			t.Fatalf("hold: %v %v", ok, err)
		}
		// later migrations roll back first; the recording one is refused while a hold exists
		for {
			m, err := s.DB().MigrateDown(ctx)
			if err != nil {
				break
			}
			if m.Name == "recording_policy" {
				t.Fatal("rollback with a legal hold was not refused")
			}
		}
		if ok, _ := s.ReleaseHold(ctx, h.ID); !ok {
			t.Fatal("release")
		}
		if m, err := s.DB().MigrateDown(ctx); err != nil || m.Name != "recording_policy" {
			t.Fatalf("rollback after release: %+v %v", m, err)
		}
	})
}

func TestRecordingInventory(t *testing.T) {
	storetest.ForEach(t, func(t *testing.T, s *store.Store) {
		ctx := context.Background()
		c := &store.Connection{Name: "legacy", Protocol: "ssh", Parameters: map[string]string{"hostname": "h",
			"recording-path": "/old", "recording-include-keys": "true", "typescript-path": "/old", "recording-exclude-mouse": "true"}}
		if err := s.CreateConnection(ctx, c); err != nil {
			t.Fatal(err)
		}
		items, err := s.ApplyRecordingInventory(ctx)
		if err != nil || len(items) != 1 || !items[0].Video || !items[0].Text || !items[0].Keys || len(items[0].Removed) != 4 {
			t.Fatalf("inventory: %+v %v", items, err)
		}
		got, _ := s.GetConnection(ctx, c.ID)
		if got.RecVideo == nil || *got.RecVideo != "on" || got.RecKeys == nil || len(got.Parameters) != 1 {
			t.Fatalf("connection after inventory: %+v %v", got, got.Parameters)
		}
		if items, _ := s.ApplyRecordingInventory(ctx); len(items) != 0 {
			t.Fatal("inventory not idempotent")
		}
	})
}

// Upgrading a populated database to recording storage keeps existing rows and gives them neutral storage state;
// rolling it back is refused while a set has a remote copy.
func TestRecordingStorageMigration(t *testing.T) {
	storetest.ForEach(t, func(t *testing.T, s *store.Store) {
		ctx := context.Background()
		db := s.DB()
		// back to 0007, then populate
		for {
			m, err := db.MigrateDown(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if m.Name == "recording_storage" {
				break
			}
		}
		// a row as 0007 writes it (StartHistory already knows the new columns)
		if _, err := db.ExecContext(ctx, `INSERT INTO connection_history (username, connection_name, protocol, tunnel_id, start_at)
			VALUES ('u', 'c', 'ssh', 't-up', 1)`); err != nil {
			t.Fatal(err)
		}
		h := &store.HistoryEntry{}
		if err := db.QueryRowContext(ctx, `SELECT id FROM connection_history WHERE tunnel_id = 't-up'`).Scan(&h.ID); err != nil {
			t.Fatal(err)
		}
		if n, err := db.MigrateUp(ctx); err != nil || n < 1 {
			t.Fatalf("upgrade: %d %v", n, err)
		}
		var vs, ts *string
		var attempts int
		if err := db.QueryRowContext(ctx, db.D.Rebind(`SELECT video_storage, text_storage, video_attempts FROM connection_history WHERE id = ?`), h.ID).Scan(&vs, &ts, &attempts); err != nil {
			t.Fatal(err)
		}
		if vs != nil || ts != nil || attempts != 0 {
			t.Fatalf("existing row after upgrade: %v %v %d", vs, ts, attempts)
		}
		if _, err := db.ExecContext(ctx, db.D.Rebind(`UPDATE connection_history SET video_storage = 'remote' WHERE id = ?`), h.ID); err != nil {
			t.Fatal(err)
		}
		for {
			m, err := db.MigrateDown(ctx)
			if err != nil {
				break
			}
			if m.Name == "recording_storage" {
				t.Fatal("rollback with a remote copy was not refused")
			}
		}
		if _, err := db.ExecContext(ctx, db.D.Rebind(`UPDATE connection_history SET video_storage = NULL WHERE id = ?`), h.ID); err != nil {
			t.Fatal(err)
		}
		if m, err := db.MigrateDown(ctx); err != nil || m.Name != "recording_storage" {
			t.Fatalf("rollback without remote copies: %+v %v", m, err)
		}
	})
}

// Leases and completed periods behave the same on every database (duplicate keys, MySQL's changed-rows count).
func TestJobLeases(t *testing.T) {
	storetest.ForEach(t, func(t *testing.T, s *store.Store) {
		ctx := context.Background()
		if ok, err := s.AcquireLease(ctx, "j", "a", "tok-a", 1000, 2000); !ok || err != nil {
			t.Fatalf("acquire: %v %v", ok, err)
		}
		if ok, err := s.AcquireLease(ctx, "j", "b", "tok-b", 1500, 9000); ok || err != nil {
			t.Fatalf("held lease taken: %v %v", ok, err)
		}
		if ok, err := s.RenewLease(ctx, "j", "tok-a", 1500, 2000); !ok || err != nil {
			t.Fatalf("renew with the same expiry: %v %v", ok, err)
		}
		if ok, err := s.AcquireLease(ctx, "j", "b", "tok-b", 3000, 9000); !ok || err != nil {
			t.Fatalf("expired lease not taken over: %v %v", ok, err)
		}
		if ok, _ := s.CompletePeriod(ctx, "j", "p1", "tok-a", 3000); ok {
			t.Fatal("stale run completed")
		}
		if ok, err := s.CompletePeriod(ctx, "j", "p1", "tok-b", 3000); !ok || err != nil {
			t.Fatalf("complete: %v %v", ok, err)
		}
		if done, err := s.PeriodDone(ctx, "j", "p1"); !done || err != nil {
			t.Fatalf("period: %v %v", done, err)
		}
		if ok, err := s.AcquireLease(ctx, "j", "c", "tok-c", 3000, 9000); !ok || err != nil {
			t.Fatalf("lease after completion: %v %v", ok, err)
		}
	})
}
