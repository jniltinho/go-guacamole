// Package access computes effective permissions and answers authorization
// questions according to the authorization matrix.
package access

import (
	"context"

	"github.com/jniltinho/go-guacamole/internal/store"
)

// Source explains where a permission comes from.
type Source struct {
	Via        string `json:"via"` // direct | group
	Group      string `json:"group,omitempty"`
	Folder     string `json:"folder,omitempty"`
	FolderID   int64  `json:"folderId,omitempty,string"`
	SubjectID  int64  `json:"-"`
	SourceType string `json:"-"`
}

// Effective is a user's effective permission set.
type Effective struct {
	UserID  int64
	System  map[string][]Source
	conns   map[int64][]store.SubjectGrant
	folders map[int64][]store.SubjectGrant
	parent  map[int64]*int64
	names   map[int64]string
}

// Resolve loads the effective permissions of a user.
func Resolve(ctx context.Context, s *store.Store, userID int64) (*Effective, error) {
	set, err := s.UserGrantSet(ctx, userID)
	if err != nil {
		return nil, err
	}
	groups, err := s.ListConnectionGroups(ctx)
	if err != nil {
		return nil, err
	}
	e := &Effective{UserID: userID, System: map[string][]Source{}, conns: set.Connections, folders: set.Groups,
		parent: map[int64]*int64{}, names: map[int64]string{}}
	for _, g := range groups {
		e.parent[g.ID] = g.ParentID
		e.names[g.ID] = g.Name
	}
	for perm, srcs := range set.System {
		for _, sg := range srcs {
			e.System[perm] = append(e.System[perm], toSource(sg, ""))
		}
	}
	return e, nil
}

func toSource(sg store.SubjectGrant, folder string) Source {
	s := Source{Via: "direct", SubjectID: sg.SourceID, SourceType: sg.SourceType, Folder: folder}
	if sg.SourceType == store.SubjectGroup {
		s.Via, s.Group = "group", sg.SourceName
	}
	return s
}

// IsAdmin reports ADMINISTER.
func (e *Effective) IsAdmin() bool { return len(e.System[store.PermAdminister]) > 0 }

// Has reports a system permission (ADMINISTER implies all).
func (e *Effective) Has(perm string) bool { return e.IsAdmin() || len(e.System[perm]) > 0 }

// SystemList returns the held system permissions.
func (e *Effective) SystemList() []string {
	if e.IsAdmin() {
		return append([]string(nil), store.SystemPermissions...)
	}
	var out []string
	for _, p := range store.SystemPermissions {
		if len(e.System[p]) > 0 {
			out = append(out, p)
		}
	}
	return out
}

// ConnectionSources returns why the user can read a connection (nil → no access).
// Administrators always get a single "admin" source.
func (e *Effective) ConnectionSources(connID int64, parentID *int64) []Source {
	if e.IsAdmin() {
		return []Source{{Via: "admin"}}
	}
	var out []Source
	for _, g := range e.conns[connID] {
		out = append(out, toSource(g, ""))
	}
	for f := parentID; f != nil; f = e.parent[*f] {
		for _, g := range e.folders[*f] {
			s := toSource(g, e.names[*f])
			s.FolderID = *f
			out = append(out, s)
		}
		if len(out) > 64 {
			break
		}
	}
	return out
}

// CanRead reports READ on a connection.
func (e *Effective) CanRead(c *store.Connection) bool {
	return len(e.ConnectionSources(c.ID, c.ParentID)) > 0
}

// CanReadFolder reports inheritable READ on a folder (or an ancestor).
func (e *Effective) CanReadFolder(id int64) bool {
	if e.IsAdmin() {
		return true
	}
	for f := &id; f != nil; f = e.parent[*f] {
		if len(e.folders[*f]) > 0 {
			return true
		}
	}
	return false
}

// CanEditConnection implements the "edit / non-recursive delete" row.
func (e *Effective) CanEditConnection(c *store.Connection) bool {
	return e.IsAdmin() || (e.Has(store.PermCreateConnection) && c.CreatedBy != nil && *c.CreatedBy == e.UserID)
}

// CanEditFolder implements the same row for folders.
func (e *Effective) CanEditFolder(g *store.ConnectionGroup) bool {
	return e.IsAdmin() || (e.Has(store.PermCreateConnection) && g.CreatedBy != nil && *g.CreatedBy == e.UserID)
}

// CanCreateIn reports whether the user can create objects in folder parent (nil = root).
func (e *Effective) CanCreateIn(parent *int64, visible func(int64) bool) bool {
	if e.IsAdmin() {
		return true
	}
	if !e.Has(store.PermCreateConnection) {
		return false
	}
	return parent == nil || visible(*parent)
}

// Ancestors returns the folder chain (root first) of a folder id.
func (e *Effective) Ancestors(id *int64) []int64 {
	var chain []int64
	for f := id; f != nil; f = e.parent[*f] {
		chain = append([]int64{*f}, chain...)
		if len(chain) > 64 {
			break
		}
	}
	return chain
}

// FolderName returns a folder's name.
func (e *Effective) FolderName(id int64) string { return e.names[id] }

// IsDescendant reports whether candidate is id itself or below it.
func (e *Effective) IsDescendant(candidate, id int64) bool {
	for f := &candidate; f != nil; f = e.parent[*f] {
		if *f == id {
			return true
		}
	}
	return false
}
