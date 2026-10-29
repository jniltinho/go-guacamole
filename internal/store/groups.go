package store

import (
	"context"
	"strings"
)

// CreateUserGroup inserts a user group.
func (s *Store) CreateUserGroup(ctx context.Context, g *UserGroup) error {
	g.CreatedAt = s.Now()
	id, err := s.insert(ctx, `INSERT INTO user_groups (name, disabled, created_at) VALUES (?,?,?)`, g.Name, g.Disabled, g.CreatedAt)
	if err != nil {
		return err
	}
	g.ID = id
	return nil
}

// GetUserGroup loads a group.
func (s *Store) GetUserGroup(ctx context.Context, id int64) (*UserGroup, error) {
	var g UserGroup
	err := s.row(ctx, `SELECT g.id, g.name, g.disabled, g.created_at, (SELECT COUNT(*) FROM user_group_members m WHERE m.group_id=g.id)
		FROM user_groups g WHERE g.id=?`, id).Scan(&g.ID, &g.Name, &g.Disabled, &g.CreatedAt, &g.MemberCount)
	if err != nil {
		return nil, mapErr(err)
	}
	return &g, nil
}

// GetUserGroupByName loads a group by name.
func (s *Store) GetUserGroupByName(ctx context.Context, name string) (*UserGroup, error) {
	var id int64
	if err := s.row(ctx, `SELECT id FROM user_groups WHERE name=?`, name).Scan(&id); err != nil {
		return nil, mapErr(err)
	}
	return s.GetUserGroup(ctx, id)
}

// ListUserGroups lists groups, optionally filtered by name.
func (s *Store) ListUserGroups(ctx context.Context, q string) ([]*UserGroup, error) {
	where, args := "", []any{}
	if q = strings.TrimSpace(strings.ToLower(q)); q != "" {
		where, args = ` WHERE LOWER(g.name) LIKE ?`, []any{"%" + q + "%"}
	}
	rows, err := s.query(ctx, `SELECT g.id, g.name, g.disabled, g.created_at, (SELECT COUNT(*) FROM user_group_members m WHERE m.group_id=g.id)
		FROM user_groups g`+where+` ORDER BY g.name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*UserGroup
	for rows.Next() {
		var g UserGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Disabled, &g.CreatedAt, &g.MemberCount); err != nil {
			return nil, err
		}
		out = append(out, &g)
	}
	return out, rows.Err()
}

// UpdateUserGroup renames / toggles a group.
func (s *Store) UpdateUserGroup(ctx context.Context, g *UserGroup) error {
	res, err := s.exec(ctx, `UPDATE user_groups SET name=?, disabled=? WHERE id=?`, g.Name, g.Disabled, g.ID)
	if err != nil {
		return err
	}
	return affected(res)
}

// DeleteUserGroup removes a group and its grants.
func (s *Store) DeleteUserGroup(ctx context.Context, id int64) error {
	return s.Tx(ctx, func(tx *Store) error {
		if err := tx.clearSubject(ctx, SubjectGroup, id); err != nil {
			return err
		}
		res, err := tx.exec(ctx, `DELETE FROM user_groups WHERE id=?`, id)
		if err != nil {
			return err
		}
		return affected(res)
	})
}

// GroupMembers returns the user ids in a group.
func (s *Store) GroupMembers(ctx context.Context, groupID int64) ([]int64, error) {
	return s.ids(ctx, `SELECT user_id FROM user_group_members WHERE group_id=? ORDER BY user_id`, groupID)
}

// SetGroupMembers replaces the members of a group.
func (s *Store) SetGroupMembers(ctx context.Context, groupID int64, userIDs []int64) error {
	return s.Tx(ctx, func(tx *Store) error {
		if _, err := tx.exec(ctx, `DELETE FROM user_group_members WHERE group_id=?`, groupID); err != nil {
			return err
		}
		for _, uid := range uniq(userIDs) {
			if _, err := tx.exec(ctx, `INSERT INTO user_group_members (group_id, user_id) VALUES (?,?)`, groupID, uid); err != nil {
				return err
			}
		}
		return nil
	})
}

// AddGroupMember adds one member (no error if already present).
func (s *Store) AddGroupMember(ctx context.Context, groupID, userID int64) error {
	var n int
	if err := s.row(ctx, `SELECT COUNT(*) FROM user_group_members WHERE group_id=? AND user_id=?`, groupID, userID).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := s.exec(ctx, `INSERT INTO user_group_members (group_id, user_id) VALUES (?,?)`, groupID, userID)
	return err
}

// GroupsOfUser returns groups the user belongs to.
func (s *Store) GroupsOfUser(ctx context.Context, userID int64) ([]*UserGroup, error) {
	rows, err := s.query(ctx, `SELECT g.id, g.name, g.disabled, g.created_at FROM user_groups g
		JOIN user_group_members m ON m.group_id=g.id WHERE m.user_id=? ORDER BY g.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*UserGroup
	for rows.Next() {
		var g UserGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Disabled, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &g)
	}
	return out, rows.Err()
}

func (s *Store) ids(ctx context.Context, query string, args ...any) ([]int64, error) {
	rows, err := s.query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func uniq(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := ids[:0:0]
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
