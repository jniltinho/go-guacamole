package access_test

import (
	"context"
	"testing"

	"github.com/jniltinho/go-guacamole/internal/access"
	"github.com/jniltinho/go-guacamole/internal/store"
	"github.com/jniltinho/go-guacamole/internal/store/storetest"
)

func TestMatrix(t *testing.T) {
	storetest.ForEach(t, func(t *testing.T, s *store.Store) {
		ctx := context.Background()
		mk := func(name string) *store.User {
			u := &store.User{Username: name, PasswordHash: "x"}
			if err := s.CreateUser(ctx, u); err != nil {
				t.Fatal(err)
			}
			return u
		}
		alice, bob, carol := mk("alice"), mk("bob"), mk("carol")
		ops := &store.UserGroup{Name: "ops"}
		_ = s.CreateUserGroup(ctx, ops)
		_ = s.SetGroupMembers(ctx, ops.ID, []int64{alice.ID})

		servers := &store.ConnectionGroup{Name: "Servers"}
		_ = s.CreateConnectionGroup(ctx, servers)
		linux := &store.ConnectionGroup{Name: "Linux", ParentID: &servers.ID, CreatedBy: &carol.ID}
		_ = s.CreateConnectionGroup(ctx, linux)
		web := &store.Connection{Name: "web", Protocol: "ssh", ParentID: &linux.ID, CreatedBy: &carol.ID}
		db := &store.Connection{Name: "db", Protocol: "ssh"}
		_ = s.CreateConnection(ctx, web)
		_ = s.CreateConnection(ctx, db)

		_ = s.SetGrants(ctx, store.SubjectGroup, ops.ID, &store.Grants{Groups: []int64{servers.ID}})
		_ = s.SetGrants(ctx, store.SubjectUser, bob.ID, &store.Grants{Connections: []int64{db.ID}, System: []string{store.PermAudit}})
		_ = s.SetGrants(ctx, store.SubjectUser, carol.ID, &store.Grants{System: []string{store.PermCreateConnection}})

		ea, _ := access.Resolve(ctx, s, alice.ID)
		eb, _ := access.Resolve(ctx, s, bob.ID)
		ec, _ := access.Resolve(ctx, s, carol.ID)

		if !ea.CanRead(web) || ea.CanRead(db) {
			t.Fatal("alice: inherited folder READ wrong")
		}
		src := ea.ConnectionSources(web.ID, web.ParentID)
		if len(src) != 1 || src[0].Group != "ops" || src[0].Folder != "Servers" {
			t.Fatalf("alice source: %+v", src)
		}
		if eb.CanRead(web) || !eb.CanRead(db) || !eb.Has(store.PermAudit) || eb.IsAdmin() {
			t.Fatal("bob: direct grant/audit wrong")
		}
		if !ec.CanEditConnection(web) || ec.CanEditConnection(db) || !ec.CanEditFolder(linux) || ec.CanEditFolder(servers) {
			t.Fatal("carol: creator edit rights wrong")
		}
		if ec.CanRead(web) {
			t.Fatal("creating does not imply READ")
		}
		if !ea.IsDescendant(linux.ID, servers.ID) || ea.IsDescendant(servers.ID, linux.ID) {
			t.Fatal("descendant check wrong")
		}

		ops.Disabled = true
		_ = s.UpdateUserGroup(ctx, ops)
		ea, _ = access.Resolve(ctx, s, alice.ID)
		if ea.CanRead(web) {
			t.Fatal("disabled group must not grant")
		}
	})
}
