package store

import "context"

// GetGrants returns the direct grants of a subject.
func (s *Store) GetGrants(ctx context.Context, subjectType string, id int64) (*Grants, error) {
	g := &Grants{System: []string{}, Connections: []int64{}, Groups: []int64{}}
	rows, err := s.query(ctx, `SELECT permission FROM system_permissions WHERE subject_type=? AND subject_id=? ORDER BY permission`, subjectType, id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return nil, err
		}
		g.System = append(g.System, p)
	}
	rows.Close()
	if g.Connections, err = s.ids(ctx, `SELECT connection_id FROM connection_permissions WHERE subject_type=? AND subject_id=? ORDER BY connection_id`, subjectType, id); err != nil {
		return nil, err
	}
	if g.Groups, err = s.ids(ctx, `SELECT group_id FROM connection_group_permissions WHERE subject_type=? AND subject_id=? ORDER BY group_id`, subjectType, id); err != nil {
		return nil, err
	}
	if g.Connections == nil {
		g.Connections = []int64{}
	}
	if g.Groups == nil {
		g.Groups = []int64{}
	}
	return g, nil
}

// SetGrants atomically replaces all grants of a subject.
func (s *Store) SetGrants(ctx context.Context, subjectType string, id int64, g *Grants) error {
	return s.Tx(ctx, func(tx *Store) error {
		if err := tx.clearSubject(ctx, subjectType, id); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, p := range g.System {
			if seen[p] {
				continue
			}
			seen[p] = true
			if _, err := tx.exec(ctx, `INSERT INTO system_permissions (subject_type, subject_id, permission) VALUES (?,?,?)`, subjectType, id, p); err != nil {
				return err
			}
		}
		for _, c := range uniq(g.Connections) {
			if _, err := tx.exec(ctx, `INSERT INTO connection_permissions (subject_type, subject_id, connection_id) VALUES (?,?,?)`, subjectType, id, c); err != nil {
				return err
			}
		}
		for _, cg := range uniq(g.Groups) {
			if _, err := tx.exec(ctx, `INSERT INTO connection_group_permissions (subject_type, subject_id, group_id) VALUES (?,?,?)`, subjectType, id, cg); err != nil {
				return err
			}
		}
		return nil
	})
}

// GrantConnection adds a single connection READ grant.
func (s *Store) GrantConnection(ctx context.Context, subjectType string, id, connID int64) error {
	_, err := s.exec(ctx, `INSERT INTO connection_permissions (subject_type, subject_id, connection_id) VALUES (?,?,?)`, subjectType, id, connID)
	if err == ErrConflict {
		return nil
	}
	return err
}

// GrantSystem adds a single system permission.
func (s *Store) GrantSystem(ctx context.Context, subjectType string, id int64, perm string) error {
	_, err := s.exec(ctx, `INSERT INTO system_permissions (subject_type, subject_id, permission) VALUES (?,?,?)`, subjectType, id, perm)
	if err == ErrConflict {
		return nil
	}
	return err
}

func (s *Store) clearSubject(ctx context.Context, subjectType string, id int64) error {
	for _, t := range []string{"system_permissions", "connection_permissions", "connection_group_permissions"} {
		if _, err := s.exec(ctx, `DELETE FROM `+t+` WHERE subject_type=? AND subject_id=?`, subjectType, id); err != nil {
			return err
		}
	}
	return nil
}

// SubjectGrant is a grant together with its source.
type SubjectGrant struct {
	SourceType string // user|group
	SourceID   int64
	SourceName string
}

// UserGrantSet is the raw material for effective permissions.
type UserGrantSet struct {
	System      map[string][]SubjectGrant
	Connections map[int64][]SubjectGrant
	Groups      map[int64][]SubjectGrant
}

// UserGrantSet returns the direct grants of the user and of each enabled group it belongs to.
func (s *Store) UserGrantSet(ctx context.Context, userID int64) (*UserGrantSet, error) {
	set := &UserGrantSet{System: map[string][]SubjectGrant{}, Connections: map[int64][]SubjectGrant{}, Groups: map[int64][]SubjectGrant{}}
	subjects := `(subject_type='user' AND subject_id=?) OR (subject_type='group' AND subject_id IN
		(SELECT g.id FROM user_groups g JOIN user_group_members m ON m.group_id=g.id WHERE m.user_id=? AND g.disabled=?))`
	names := `CASE WHEN p.subject_type='user' THEN (SELECT username FROM users WHERE id=p.subject_id)
		ELSE (SELECT name FROM user_groups WHERE id=p.subject_id) END`
	load := func(q string, fn func(key string, keyID int64, g SubjectGrant)) error {
		rows, err := s.query(ctx, q, userID, userID, false)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			var keyID int64
			var g SubjectGrant
			if err := rows.Scan(&key, &keyID, &g.SourceType, &g.SourceID, &g.SourceName); err != nil {
				return err
			}
			fn(key, keyID, g)
		}
		return rows.Err()
	}
	if err := load(`SELECT p.permission, 0, p.subject_type, p.subject_id, `+names+` FROM system_permissions p WHERE `+subjects,
		func(k string, _ int64, g SubjectGrant) { set.System[k] = append(set.System[k], g) }); err != nil {
		return nil, err
	}
	if err := load(`SELECT '', p.connection_id, p.subject_type, p.subject_id, `+names+` FROM connection_permissions p WHERE `+subjects,
		func(_ string, id int64, g SubjectGrant) { set.Connections[id] = append(set.Connections[id], g) }); err != nil {
		return nil, err
	}
	if err := load(`SELECT '', p.group_id, p.subject_type, p.subject_id, `+names+` FROM connection_group_permissions p WHERE `+subjects,
		func(_ string, id int64, g SubjectGrant) { set.Groups[id] = append(set.Groups[id], g) }); err != nil {
		return nil, err
	}
	return set, nil
}

// PermissionCount summarizes grants for the permission overview.
type PermissionCount struct {
	SubjectType string   `json:"type"`
	ID          int64    `json:"id,string"`
	Name        string   `json:"name"`
	Count       int      `json:"permissions"`
	InheritFrom []string `json:"inheritedFrom"`
}

// PermissionOverview lists users and groups with their direct grant counts.
func (s *Store) PermissionOverview(ctx context.Context, limit int) ([]PermissionCount, error) {
	count := `(SELECT COUNT(*) FROM system_permissions x WHERE x.subject_type=? AND x.subject_id=t.id) +
		(SELECT COUNT(*) FROM connection_permissions x WHERE x.subject_type=? AND x.subject_id=t.id) +
		(SELECT COUNT(*) FROM connection_group_permissions x WHERE x.subject_type=? AND x.subject_id=t.id)`
	var out []PermissionCount
	rows, err := s.query(ctx, `SELECT t.id, t.username, `+count+` FROM users t ORDER BY t.username LIMIT ?`, "user", "user", "user", limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		pc := PermissionCount{SubjectType: SubjectUser}
		if err := rows.Scan(&pc.ID, &pc.Name, &pc.Count); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, pc)
	}
	rows.Close()
	for i := range out {
		gs, err := s.GroupsOfUser(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].InheritFrom = []string{}
		for _, g := range gs {
			if !g.Disabled {
				out[i].InheritFrom = append(out[i].InheritFrom, g.Name)
			}
		}
	}
	rows, err = s.query(ctx, `SELECT t.id, t.name, `+count+` FROM user_groups t ORDER BY t.name LIMIT ?`, "group", "group", "group", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		pc := PermissionCount{SubjectType: SubjectGroup, InheritFrom: []string{}}
		if err := rows.Scan(&pc.ID, &pc.Name, &pc.Count); err != nil {
			return nil, err
		}
		out = append(out, pc)
	}
	return out, rows.Err()
}
