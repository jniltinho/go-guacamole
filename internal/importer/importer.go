// Package importer copies users, groups, connections and permissions from an
// official Apache Guacamole JDBC database (MySQL/MariaDB or PostgreSQL) into
// GoGuacamole. The source is only ever read inside a READ ONLY transaction.
package importer

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/jniltinho/go-guacamole/internal/access"
	"github.com/jniltinho/go-guacamole/internal/store"
)

// Options controls an import run.
type Options struct {
	SourceDriver    string // mysql | postgres
	DryRun          bool
	Update          bool // --conflict update
	AcceptExpansion bool
	Out             io.Writer
}

// Report summarizes an import.
type Report struct {
	Created  map[string]int
	Updated  map[string]int
	Skipped  map[string]int
	Warnings []string
	Diffs    []string
}

func newReport() *Report {
	return &Report{Created: map[string]int{}, Updated: map[string]int{}, Skipped: map[string]int{}}
}

func (r *Report) warn(format string, a ...any) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, a...))
}

type srcUser struct {
	id, entity                          int64
	name, hash, salt                    string
	disabled, expired                   bool
	validFrom, validUntil, wStart, wEnd sql.NullString
	tz, fullName, email                 sql.NullString
}

type srcGroup struct {
	id, entity int64
	name       string
	disabled   bool
}

type srcFolder struct {
	id     int64
	parent sql.NullInt64
	name   string
	typ    string
}

type srcConn struct {
	id                  int64
	name                string
	parent              sql.NullInt64
	protocol            string
	proxyPort           sql.NullInt64
	proxyHost, proxyEnc sql.NullString
	maxConn, maxPerUser sql.NullInt64
	params              map[string]string
}

// Run executes the import.
func Run(ctx context.Context, src *sql.DB, dst *store.Store, opt Options) (*Report, error) {
	rep := newReport()
	stx, err := src.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin read-only source transaction: %w", err)
	}
	defer stx.Rollback() //nolint:errcheck // read-only

	castText := "CAST(%s AS CHAR)"
	if opt.SourceDriver == "postgres" {
		castText = "CAST(%s AS TEXT)"
	}
	ct := func(col string) string { return fmt.Sprintf(castText, col) }

	// ---- read source ----
	users := map[int64]*srcUser{} // by entity
	rows, err := stx.QueryContext(ctx, `SELECT u.user_id, e.entity_id, e.name, u.password_hash, u.password_salt, u.disabled, u.expired,
		`+ct("u.valid_from")+`, `+ct("u.valid_until")+`, `+ct("u.access_window_start")+`, `+ct("u.access_window_end")+`,
		u.timezone, u.full_name, u.email_address
		FROM guacamole_user u JOIN guacamole_entity e ON e.entity_id = u.entity_id`)
	if err != nil {
		return nil, fmt.Errorf("read guacamole_user: %w", err)
	}
	for rows.Next() {
		var u srcUser
		var hash, salt []byte
		if err := rows.Scan(&u.id, &u.entity, &u.name, &hash, &salt, &u.disabled, &u.expired, &u.validFrom, &u.validUntil,
			&u.wStart, &u.wEnd, &u.tz, &u.fullName, &u.email); err != nil {
			rows.Close()
			return nil, err
		}
		u.hash, u.salt = hex.EncodeToString(hash), hex.EncodeToString(salt)
		users[u.entity] = &u
	}
	rows.Close()

	groups := map[int64]*srcGroup{} // by entity
	rows, err = stx.QueryContext(ctx, `SELECT g.user_group_id, e.entity_id, e.name, g.disabled
		FROM guacamole_user_group g JOIN guacamole_entity e ON e.entity_id = g.entity_id`)
	if err != nil {
		return nil, fmt.Errorf("read guacamole_user_group: %w", err)
	}
	groupByID := map[int64]*srcGroup{}
	for rows.Next() {
		var g srcGroup
		if err := rows.Scan(&g.id, &g.entity, &g.name, &g.disabled); err != nil {
			rows.Close()
			return nil, err
		}
		groups[g.entity] = &g
		groupByID[g.id] = &g
	}
	rows.Close()

	// member entity → groups containing it
	parentsOf := map[int64][]*srcGroup{}
	rows, err = stx.QueryContext(ctx, `SELECT user_group_id, member_entity_id FROM guacamole_user_group_member`)
	if err != nil {
		return nil, fmt.Errorf("read guacamole_user_group_member: %w", err)
	}
	nested := 0
	for rows.Next() {
		var gid, member int64
		if err := rows.Scan(&gid, &member); err != nil {
			rows.Close()
			return nil, err
		}
		if g := groupByID[gid]; g != nil {
			parentsOf[member] = append(parentsOf[member], g)
			if groups[member] != nil {
				nested++
			}
		}
	}
	rows.Close()
	if nested > 0 {
		rep.warn("%d nested group membership(s) flattened (paths through disabled groups grant nothing)", nested)
	}

	folders := map[int64]*srcFolder{}
	rows, err = stx.QueryContext(ctx, `SELECT connection_group_id, parent_id, connection_group_name, `+ct("type")+` FROM guacamole_connection_group`)
	if err != nil {
		return nil, fmt.Errorf("read guacamole_connection_group: %w", err)
	}
	balancing := 0
	for rows.Next() {
		var f srcFolder
		if err := rows.Scan(&f.id, &f.parent, &f.name, &f.typ); err != nil {
			rows.Close()
			return nil, err
		}
		if f.typ == "BALANCING" {
			balancing++
		}
		folders[f.id] = &f
	}
	rows.Close()
	if balancing > 0 {
		rep.warn("%d balancing group(s) imported as organizational", balancing)
	}

	conns := map[int64]*srcConn{}
	rows, err = stx.QueryContext(ctx, `SELECT connection_id, connection_name, parent_id, protocol, proxy_port, proxy_hostname, `+
		ct("proxy_encryption_method")+`, max_connections, max_connections_per_user FROM guacamole_connection`)
	if err != nil {
		return nil, fmt.Errorf("read guacamole_connection: %w", err)
	}
	for rows.Next() {
		c := srcConn{params: map[string]string{}}
		if err := rows.Scan(&c.id, &c.name, &c.parent, &c.protocol, &c.proxyPort, &c.proxyHost, &c.proxyEnc, &c.maxConn, &c.maxPerUser); err != nil {
			rows.Close()
			return nil, err
		}
		conns[c.id] = &c
	}
	rows.Close()
	rows, err = stx.QueryContext(ctx, `SELECT connection_id, parameter_name, parameter_value FROM guacamole_connection_parameter`)
	if err != nil {
		return nil, fmt.Errorf("read guacamole_connection_parameter: %w", err)
	}
	for rows.Next() {
		var id int64
		var n, v string
		if err := rows.Scan(&id, &n, &v); err != nil {
			rows.Close()
			return nil, err
		}
		if c := conns[id]; c != nil {
			c.params[n] = v
		}
	}
	rows.Close()

	type grant struct{ entity, obj int64 }
	var sysGrants []struct {
		entity int64
		perm   string
	}
	rows, err = stx.QueryContext(ctx, `SELECT entity_id, `+ct("permission")+` FROM guacamole_system_permission`)
	if err != nil {
		return nil, fmt.Errorf("read guacamole_system_permission: %w", err)
	}
	skippedSys := 0
	for rows.Next() {
		var g struct {
			entity int64
			perm   string
		}
		if err := rows.Scan(&g.entity, &g.perm); err != nil {
			rows.Close()
			return nil, err
		}
		switch g.perm {
		case store.PermAdminister, store.PermCreateConnection, store.PermCreateUser, store.PermAudit:
			sysGrants = append(sysGrants, g)
		default:
			skippedSys++
		}
	}
	rows.Close()
	if skippedSys > 0 {
		rep.warn("skipped %d system permission(s) without equivalent (e.g. CREATE_CONNECTION_GROUP, CREATE_USER_GROUP)", skippedSys)
	}

	var connRead []grant
	otherConnPerms := 0
	rows, err = stx.QueryContext(ctx, `SELECT entity_id, connection_id, `+ct("permission")+` FROM guacamole_connection_permission`)
	if err != nil {
		return nil, fmt.Errorf("read guacamole_connection_permission: %w", err)
	}
	for rows.Next() {
		var g grant
		var perm string
		if err := rows.Scan(&g.entity, &g.obj, &perm); err != nil {
			rows.Close()
			return nil, err
		}
		if perm == "READ" {
			connRead = append(connRead, g)
		} else {
			otherConnPerms++
		}
	}
	rows.Close()

	var folderRead []grant
	rows, err = stx.QueryContext(ctx, `SELECT entity_id, connection_group_id FROM guacamole_connection_group_permission WHERE `+ct("permission")+` = 'READ'`)
	if err != nil {
		return nil, fmt.Errorf("read guacamole_connection_group_permission: %w", err)
	}
	for rows.Next() {
		var g grant
		if err := rows.Scan(&g.entity, &g.obj); err != nil {
			rows.Close()
			return nil, err
		}
		folderRead = append(folderRead, g)
	}
	rows.Close()
	if otherConnPerms > 0 {
		rep.warn("skipped %d connection permission(s) other than READ (UPDATE/DELETE/ADMINISTER)", otherConnPerms)
	}
	var sharing int
	if err := stx.QueryRowContext(ctx, `SELECT COUNT(*) FROM guacamole_sharing_profile`).Scan(&sharing); err == nil && sharing > 0 {
		rep.warn("skipped %d sharing profile(s) (not supported)", sharing)
	}

	// ---- effective source groups per user (flattened) ----
	effGroups := func(entity int64) []*srcGroup {
		seen := map[int64]bool{}
		var out []*srcGroup
		var walk func(member int64, viaEnabled bool)
		walk = func(member int64, viaEnabled bool) {
			for _, g := range parentsOf[member] {
				if seen[g.entity] || !viaEnabled {
					continue
				}
				seen[g.entity] = true
				out = append(out, g)
				walk(g.entity, !g.disabled)
			}
		}
		walk(entity, true)
		return out
	}

	// ---- write target ----
	var aborted = errors.New("dry run")
	err = dst.Tx(ctx, func(tx *store.Store) error {
		userMap := map[int64]int64{}  // src entity → dst user id
		groupMap := map[int64]int64{} // src entity → dst group id
		for _, su := range sortedUsers(users) {
			u, err := tx.GetUserByName(ctx, su.name)
			exists := err == nil
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			if exists && !opt.Update {
				rep.Skipped["users"]++
				userMap[su.entity] = u.ID
				continue
			}
			if !exists {
				u = &store.User{Username: su.name}
			}
			u.Disabled, u.MustChangePassword = su.disabled, su.expired
			u.ValidFrom, u.ValidUntil = clip(su.validFrom, 10), clip(su.validUntil, 10)
			u.AccessWindowStart, u.AccessWindowEnd = clip(su.wStart, 8), clip(su.wEnd, 8)
			u.Timezone, u.FullName, u.Email = nstr(su.tz), nstr(su.fullName), nstr(su.email)
			if exists {
				if err := tx.UpdateUser(ctx, u); err != nil {
					return err
				}
				if err := tx.SetPassword(ctx, u.ID, su.hash, store.SchemeGuacSHA256, su.salt, su.expired); err != nil {
					return err
				}
				rep.Updated["users"]++
			} else {
				u.PasswordHash, u.PasswordScheme, u.PasswordSalt = su.hash, store.SchemeGuacSHA256, su.salt
				if err := tx.CreateUser(ctx, u); err != nil {
					return err
				}
				rep.Created["users"]++
			}
			userMap[su.entity] = u.ID
		}
		for _, sg := range sortedGroups(groups) {
			g, err := tx.GetUserGroupByName(ctx, sg.name)
			exists := err == nil
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			switch {
			case exists && !opt.Update:
				rep.Skipped["user groups"]++
			case exists:
				g.Disabled = sg.disabled
				if err := tx.UpdateUserGroup(ctx, g); err != nil {
					return err
				}
				rep.Updated["user groups"]++
			default:
				g = &store.UserGroup{Name: sg.name, Disabled: sg.disabled}
				if err := tx.CreateUserGroup(ctx, g); err != nil {
					return err
				}
				rep.Created["user groups"]++
			}
			groupMap[sg.entity] = g.ID
		}
		for entity, uid := range userMap {
			for _, g := range effGroups(entity) {
				if err := tx.AddGroupMember(ctx, groupMap[g.entity], uid); err != nil {
					return err
				}
			}
		}

		// folders in topological order, matched by path
		folderMap := map[int64]int64{}
		existingFolders, err := tx.ListConnectionGroups(ctx)
		if err != nil {
			return err
		}
		byKey := map[string]int64{}
		for _, f := range existingFolders {
			byKey[parentKey(f.ParentID)+"/"+f.Name] = f.ID
		}
		var place func(f *srcFolder) (int64, error)
		place = func(f *srcFolder) (int64, error) {
			if id, ok := folderMap[f.id]; ok {
				return id, nil
			}
			var parent *int64
			if f.parent.Valid {
				pf := folders[f.parent.Int64]
				if pf == nil {
					return 0, fmt.Errorf("folder %d has unknown parent", f.id)
				}
				pid, err := place(pf)
				if err != nil {
					return 0, err
				}
				parent = &pid
			}
			key := parentKey(parent) + "/" + f.name
			if id, ok := byKey[key]; ok {
				rep.Skipped["connection groups"]++
				folderMap[f.id] = id
				return id, nil
			}
			g := &store.ConnectionGroup{Name: f.name, ParentID: parent}
			if err := tx.CreateConnectionGroup(ctx, g); err != nil {
				return 0, err
			}
			rep.Created["connection groups"]++
			byKey[key] = g.ID
			folderMap[f.id] = g.ID
			return g.ID, nil
		}
		for _, f := range sortedFolders(folders) {
			if _, err := place(f); err != nil {
				return err
			}
		}

		existingConns, err := tx.ListConnections(ctx)
		if err != nil {
			return err
		}
		connKey := map[string]int64{}
		for _, c := range existingConns {
			connKey[parentKey(c.ParentID)+"/"+c.Name] = c.ID
		}
		connMap := map[int64]int64{}
		for _, sc := range sortedConns(conns) {
			var parent *int64
			if sc.parent.Valid {
				pid := folderMap[sc.parent.Int64]
				parent = &pid
			}
			c := &store.Connection{Name: sc.name, Protocol: sc.protocol, ParentID: parent, Parameters: sc.params,
				GuacdHost: nstr(sc.proxyHost), GuacdPort: nint(sc.proxyPort), GuacdEncryption: nstr(sc.proxyEnc),
				MaxConnections: nint(sc.maxConn), MaxConnectionsPerUser: nint(sc.maxPerUser)}
			key := parentKey(parent) + "/" + sc.name
			if id, ok := connKey[key]; ok {
				connMap[sc.id] = id
				if !opt.Update {
					rep.Skipped["connections"]++
					continue
				}
				c.ID = id
				if err := tx.UpdateConnection(ctx, c); err != nil {
					return err
				}
				rep.Updated["connections"]++
				continue
			}
			if err := tx.CreateConnection(ctx, c); err != nil {
				return err
			}
			connMap[sc.id] = c.ID
			connKey[key] = c.ID
			rep.Created["connections"]++
		}

		subject := func(entity int64) (string, int64, bool) {
			if id, ok := userMap[entity]; ok {
				return store.SubjectUser, id, true
			}
			if id, ok := groupMap[entity]; ok {
				return store.SubjectGroup, id, true
			}
			return "", 0, false
		}
		for _, g := range sysGrants {
			if typ, id, ok := subject(g.entity); ok {
				if err := tx.GrantSystem(ctx, typ, id, g.perm); err != nil {
					return err
				}
				rep.Created["system permissions"]++
			}
		}
		for _, g := range connRead {
			typ, id, ok := subject(g.entity)
			cid, ok2 := connMap[g.obj]
			if ok && ok2 {
				if err := tx.GrantConnection(ctx, typ, id, cid); err != nil {
					return err
				}
				rep.Created["connection READ grants"]++
			}
		}
		skippedFolder, skippedBalancing := 0, 0
		for _, g := range folderRead {
			f := folders[g.obj]
			if f == nil || f.typ != "BALANCING" {
				skippedFolder++
				continue
			}
			if !opt.AcceptExpansion {
				skippedBalancing++
				continue
			}
			typ, id, ok := subject(g.entity)
			if !ok {
				continue
			}
			for _, sc := range conns {
				if sc.parent.Valid && sc.parent.Int64 == f.id {
					if err := tx.GrantConnection(ctx, typ, id, connMap[sc.id]); err != nil {
						return err
					}
					rep.Created["connection READ grants (from balancing groups)"]++
				}
			}
		}
		if skippedFolder > 0 {
			rep.warn("skipped %d connection-group READ grant(s): upstream they do not grant child connections", skippedFolder)
		}
		if skippedBalancing > 0 {
			rep.warn("skipped %d balancing-group READ grant(s); re-run with --accept-expansion to grant READ on each child connection", skippedBalancing)
		}

		// ---- access diff (source launchable set vs target effective set) ----
		for _, su := range sortedUsers(users) {
			srcSet := map[int64]bool{}
			entities := []int64{su.entity}
			for _, g := range effGroups(su.entity) {
				if !g.disabled {
					entities = append(entities, g.entity)
				}
			}
			isAdmin := false
			for _, e := range entities {
				for _, g := range connRead {
					if g.entity == e {
						srcSet[connMap[g.obj]] = true
					}
				}
				for _, g := range sysGrants {
					if g.entity == e && g.perm == store.PermAdminister {
						isAdmin = true
					}
				}
			}
			uid := userMap[su.entity]
			eff, err := access.Resolve(ctx, tx, uid)
			if err != nil {
				return err
			}
			if isAdmin != eff.IsAdmin() {
				if eff.IsAdmin() {
					rep.Diffs = append(rep.Diffs, fmt.Sprintf("%s: GAINED ADMINISTER (existing target account)", su.name))
				} else {
					rep.Diffs = append(rep.Diffs, fmt.Sprintf("%s: LOST ADMINISTER", su.name))
				}
			}
			all, err := tx.ListConnections(ctx)
			if err != nil {
				return err
			}
			var gained, lost []string
			for _, c := range all {
				can := eff.CanRead(c)
				had := isAdmin || srcSet[c.ID]
				if can && !had {
					gained = append(gained, c.Name)
				}
				if !can && had {
					lost = append(lost, c.Name)
				}
			}
			if len(gained) > 0 {
				rep.Diffs = append(rep.Diffs, fmt.Sprintf("%s: GAINED %s", su.name, strings.Join(gained, ", ")))
			}
			if len(lost) > 0 {
				rep.Diffs = append(rep.Diffs, fmt.Sprintf("%s: LOST %s", su.name, strings.Join(lost, ", ")))
			}
		}
		if opt.DryRun {
			return aborted
		}
		return nil
	})
	if err != nil && !errors.Is(err, aborted) {
		return rep, err
	}
	return rep, nil
}

// Print writes a human-readable report.
func (r *Report) Print(w io.Writer, dry bool) {
	if dry {
		fmt.Fprintln(w, "DRY RUN — nothing was written")
	}
	keys := map[string]bool{}
	for _, m := range []map[string]int{r.Created, r.Updated, r.Skipped} {
		for k := range m {
			keys[k] = true
		}
	}
	var names []string
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	fmt.Fprintf(w, "%-48s %8s %8s %8s\n", "OBJECT", "CREATED", "UPDATED", "SKIPPED")
	for _, k := range names {
		fmt.Fprintf(w, "%-48s %8d %8d %8d\n", k, r.Created[k], r.Updated[k], r.Skipped[k])
	}
	for _, wmsg := range r.Warnings {
		fmt.Fprintln(w, "WARNING:", wmsg)
	}
	if len(r.Diffs) == 0 {
		fmt.Fprintln(w, "access check: every user can launch exactly the same connections as in the source")
	}
	for _, d := range r.Diffs {
		fmt.Fprintln(w, "ACCESS DIFF:", d)
	}
}

func parentKey(p *int64) string {
	if p == nil {
		return "ROOT"
	}
	return strconv.FormatInt(*p, 10)
}

func nstr(ns sql.NullString) *string {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	v := ns.String
	return &v
}

func clip(ns sql.NullString, n int) *string {
	p := nstr(ns)
	if p != nil && len(*p) > n {
		v := (*p)[:n]
		return &v
	}
	return p
}

func nint(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func sortedUsers(m map[int64]*srcUser) []*srcUser {
	out := make([]*srcUser, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func sortedGroups(m map[int64]*srcGroup) []*srcGroup {
	out := make([]*srcGroup, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func sortedFolders(m map[int64]*srcFolder) []*srcFolder {
	out := make([]*srcFolder, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

func sortedConns(m map[int64]*srcConn) []*srcConn {
	out := make([]*srcConn, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}
