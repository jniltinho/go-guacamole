package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jniltinho/go-guacamole/internal/secrets"
)

// CreateConnectionGroup inserts a folder, enforcing unique names per parent.
func (s *Store) CreateConnectionGroup(ctx context.Context, g *ConnectionGroup) error {
	return s.Tx(ctx, func(tx *Store) error {
		if err := tx.checkName(ctx, "connection_groups", g.ParentID, g.Name, 0); err != nil {
			return err
		}
		g.CreatedAt = tx.Now()
		id, err := tx.insert(ctx, `INSERT INTO connection_groups (parent_id, name, created_by, created_at) VALUES (?,?,?,?)`,
			nullInt64(g.ParentID), g.Name, nullInt64(g.CreatedBy), g.CreatedAt)
		if err != nil {
			return err
		}
		g.ID = id
		return nil
	})
}

// ListConnectionGroups returns every folder.
func (s *Store) ListConnectionGroups(ctx context.Context) ([]*ConnectionGroup, error) {
	rows, err := s.query(ctx, `SELECT id, parent_id, name, created_by, created_at FROM connection_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ConnectionGroup
	for rows.Next() {
		var g ConnectionGroup
		var parent, creator sql.NullInt64
		if err := rows.Scan(&g.ID, &parent, &g.Name, &creator, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.ParentID, g.CreatedBy = int64Ptr(parent), int64Ptr(creator)
		out = append(out, &g)
	}
	return out, rows.Err()
}

// GetConnectionGroup loads one folder.
func (s *Store) GetConnectionGroup(ctx context.Context, id int64) (*ConnectionGroup, error) {
	var g ConnectionGroup
	var parent, creator sql.NullInt64
	err := s.row(ctx, `SELECT id, parent_id, name, created_by, created_at FROM connection_groups WHERE id=?`, id).
		Scan(&g.ID, &parent, &g.Name, &creator, &g.CreatedAt)
	if err != nil {
		return nil, mapErr(err)
	}
	g.ParentID, g.CreatedBy = int64Ptr(parent), int64Ptr(creator)
	return &g, nil
}

// UpdateConnectionGroup renames/moves a folder.
func (s *Store) UpdateConnectionGroup(ctx context.Context, g *ConnectionGroup) error {
	return s.Tx(ctx, func(tx *Store) error {
		if err := tx.checkName(ctx, "connection_groups", g.ParentID, g.Name, g.ID); err != nil {
			return err
		}
		res, err := tx.exec(ctx, `UPDATE connection_groups SET parent_id=?, name=? WHERE id=?`, nullInt64(g.ParentID), g.Name, g.ID)
		if err != nil {
			return err
		}
		return affected(res)
	})
}

// DeleteConnectionGroup deletes a folder (cascading to descendants).
func (s *Store) DeleteConnectionGroup(ctx context.Context, id int64) error {
	res, err := s.exec(ctx, `DELETE FROM connection_groups WHERE id=?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// GroupIsEmpty reports whether a folder has no children.
func (s *Store) GroupIsEmpty(ctx context.Context, id int64) (bool, error) {
	var n int
	err := s.row(ctx, `SELECT (SELECT COUNT(*) FROM connections WHERE parent_id=?) + (SELECT COUNT(*) FROM connection_groups WHERE parent_id=?)`, id, id).Scan(&n)
	return n == 0, err
}

func (s *Store) checkName(ctx context.Context, table string, parent *int64, name string, selfID int64) error {
	var n int
	var err error
	if parent == nil {
		err = s.row(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE parent_id IS NULL AND name=? AND id<>?`, table), name, selfID).Scan(&n)
	} else {
		err = s.row(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE parent_id=? AND name=? AND id<>?`, table), *parent, name, selfID).Scan(&n)
	}
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrConflict
	}
	return nil
}

const connCols = `id, parent_id, name, protocol, guacd_host, guacd_port, guacd_encryption, max_connections,
	max_connections_per_user, created_by, created_at, updated_at, rec_video, rec_text, rec_keys,
	retention_video_days, retention_text_days, rec_version, disabled`

func scanConn(sc interface{ Scan(...any) error }) (*Connection, error) {
	var c Connection
	var parent, port, maxc, maxu, creator, rv, rt sql.NullInt64
	var host, enc, recV, recT, recK sql.NullString
	if err := sc.Scan(&c.ID, &parent, &c.Name, &c.Protocol, &host, &port, &enc, &maxc, &maxu, &creator, &c.CreatedAt, &c.UpdatedAt,
		&recV, &recT, &recK, &rv, &rt, &c.RecVersion, &c.Disabled); err != nil {
		return nil, mapErr(err)
	}
	c.RecVideo, c.RecText, c.RecKeys = strPtr(recV), strPtr(recT), strPtr(recK)
	c.RetentionVideoDays, c.RetentionTextDays = intPtr(rv), intPtr(rt)
	c.ParentID, c.CreatedBy = int64Ptr(parent), int64Ptr(creator)
	c.GuacdHost, c.GuacdPort, c.GuacdEncryption = strPtr(host), intPtr(port), strPtr(enc)
	c.MaxConnections, c.MaxConnectionsPerUser = intPtr(maxc), intPtr(maxu)
	return &c, nil
}

// CreateConnection inserts a connection with its parameters.
func (s *Store) CreateConnection(ctx context.Context, c *Connection) error {
	return s.Tx(ctx, func(tx *Store) error {
		if err := tx.checkName(ctx, "connections", c.ParentID, c.Name, 0); err != nil {
			return err
		}
		now := tx.Now()
		c.CreatedAt, c.UpdatedAt = now, now
		id, err := tx.insert(ctx, `INSERT INTO connections (parent_id, name, protocol, guacd_host, guacd_port, guacd_encryption,
			max_connections, max_connections_per_user, disabled, created_by, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			nullInt64(c.ParentID), c.Name, c.Protocol, nullStr(c.GuacdHost), nullInt(c.GuacdPort), nullStr(c.GuacdEncryption),
			nullInt(c.MaxConnections), nullInt(c.MaxConnectionsPerUser), c.Disabled, nullInt64(c.CreatedBy), now, now)
		if err != nil {
			return err
		}
		c.ID = id
		return tx.writeParams(ctx, c.ID, c.Protocol, c.Parameters)
	})
}

// UpdateConnectionUnprivileged saves what a non-administrator may change: the
// protocol and guacd endpoint stored in the database are never overwritten.
func (s *Store) UpdateConnectionUnprivileged(ctx context.Context, c *Connection) error {
	return s.Tx(ctx, func(tx *Store) error {
		if err := tx.checkName(ctx, "connections", c.ParentID, c.Name, c.ID); err != nil {
			return err
		}
		c.UpdatedAt = tx.Now()
		res, err := tx.exec(ctx, `UPDATE connections SET parent_id=?, name=?, max_connections=?, max_connections_per_user=?, disabled=?,
			updated_at=? WHERE id=? AND protocol=?`,
			nullInt64(c.ParentID), c.Name, nullInt(c.MaxConnections), nullInt(c.MaxConnectionsPerUser), c.Disabled, c.UpdatedAt, c.ID, c.Protocol)
		if err != nil {
			return err
		}
		if err := affected(res); err != nil {
			return err // protocol changed meanwhile: parameters would be sealed for the wrong protocol
		}
		if _, err := tx.exec(ctx, `DELETE FROM connection_parameters WHERE connection_id=?`, c.ID); err != nil {
			return err
		}
		return tx.writeParams(ctx, c.ID, c.Protocol, c.Parameters)
	})
}

// UpdateConnection saves a connection; Parameters replaces all parameters.
func (s *Store) UpdateConnection(ctx context.Context, c *Connection) error {
	return s.Tx(ctx, func(tx *Store) error {
		if err := tx.checkName(ctx, "connections", c.ParentID, c.Name, c.ID); err != nil {
			return err
		}
		c.UpdatedAt = tx.Now()
		res, err := tx.exec(ctx, `UPDATE connections SET parent_id=?, name=?, protocol=?, guacd_host=?, guacd_port=?, guacd_encryption=?,
			max_connections=?, max_connections_per_user=?, disabled=?, updated_at=? WHERE id=?`,
			nullInt64(c.ParentID), c.Name, c.Protocol, nullStr(c.GuacdHost), nullInt(c.GuacdPort), nullStr(c.GuacdEncryption),
			nullInt(c.MaxConnections), nullInt(c.MaxConnectionsPerUser), c.Disabled, c.UpdatedAt, c.ID)
		if err != nil {
			return err
		}
		if err := affected(res); err != nil {
			return err
		}
		if _, err := tx.exec(ctx, `DELETE FROM connection_parameters WHERE connection_id=?`, c.ID); err != nil {
			return err
		}
		return tx.writeParams(ctx, c.ID, c.Protocol, c.Parameters)
	})
}

func (s *Store) writeParams(ctx context.Context, id int64, protocol string, params map[string]string) error {
	for name, val := range params {
		if val == "" {
			continue
		}
		if secrets.IsSecret(protocol, name) {
			sealed, err := s.box.Seal(val)
			if err != nil {
				return fmt.Errorf("encrypt parameter %s: %w", name, err)
			}
			val = sealed
		}
		if _, err := s.exec(ctx, `INSERT INTO connection_parameters (connection_id, name, value) VALUES (?,?,?)`, id, name, val); err != nil {
			return err
		}
	}
	return nil
}

// GetConnection loads a connection with decrypted parameters.
func (s *Store) GetConnection(ctx context.Context, id int64) (*Connection, error) {
	c, err := scanConn(s.row(ctx, `SELECT `+connCols+` FROM connections WHERE id=?`, id))
	if err != nil {
		return nil, err
	}
	c.Parameters, err = s.ConnectionParams(ctx, id)
	return c, err
}

// ConnectionParams returns the decrypted parameters of a connection.
func (s *Store) ConnectionParams(ctx context.Context, id int64) (map[string]string, error) {
	rows, err := s.query(ctx, `SELECT name, value FROM connection_parameters WHERE connection_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var n, v string
		if err := rows.Scan(&n, &v); err != nil {
			return nil, err
		}
		if secrets.IsSealed(v) {
			if v, err = s.box.Open(v); err != nil {
				return nil, err
			}
		}
		out[n] = v
	}
	return out, rows.Err()
}

// ListConnections returns every connection with its non-secret
// hostname/port parameters loaded (for catalog and probing).
func (s *Store) ListConnections(ctx context.Context) ([]*Connection, error) {
	rows, err := s.query(ctx, `SELECT `+connCols+` FROM connections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var out []*Connection
	byID := map[int64]*Connection{}
	for rows.Next() {
		c, err := scanConn(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		c.Parameters = map[string]string{}
		out = append(out, c)
		byID[c.ID] = c
	}
	rows.Close()
	prow, err := s.query(ctx, `SELECT connection_id, name, value FROM connection_parameters WHERE name IN ('hostname','port')`)
	if err != nil {
		return nil, err
	}
	defer prow.Close()
	for prow.Next() {
		var id int64
		var n, v string
		if err := prow.Scan(&id, &n, &v); err != nil {
			return nil, err
		}
		if c := byID[id]; c != nil {
			c.Parameters[n] = v
		}
	}
	return out, prow.Err()
}

// DeleteConnection removes a connection.
func (s *Store) DeleteConnection(ctx context.Context, id int64) error {
	res, err := s.exec(ctx, `DELETE FROM connections WHERE id=?`, id)
	if err != nil {
		return err
	}
	return affected(res)
}

// HasSealedSecrets reports whether any encrypted parameter exists.
func (s *Store) HasSealedSecrets(ctx context.Context) (string, error) {
	var v string
	err := s.row(ctx, `SELECT value FROM connection_parameters WHERE value LIKE 'v1:%' LIMIT 1`).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// Favorites returns the favorite connection ids of a user.
func (s *Store) Favorites(ctx context.Context, userID int64) ([]int64, error) {
	return s.ids(ctx, `SELECT connection_id FROM favorites WHERE user_id=?`, userID)
}

// AddFavorite marks a connection as favorite.
func (s *Store) AddFavorite(ctx context.Context, userID, connID int64) error {
	_, err := s.exec(ctx, `INSERT INTO favorites (user_id, connection_id, created_at) VALUES (?,?,?)`, userID, connID, s.Now())
	if err == ErrConflict {
		return nil
	}
	return err
}

// RemoveFavorite unmarks a favorite.
func (s *Store) RemoveFavorite(ctx context.Context, userID, connID int64) error {
	_, err := s.exec(ctx, `DELETE FROM favorites WHERE user_id=? AND connection_id=?`, userID, connID)
	return err
}
