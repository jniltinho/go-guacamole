package store

import "errors"

// Common errors.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

// Subject types for permission grants.
const (
	SubjectUser  = "user"
	SubjectGroup = "group"
)

// System permissions.
const (
	PermAdminister       = "ADMINISTER"
	PermCreateConnection = "CREATE_CONNECTION"
	PermCreateUser       = "CREATE_USER"
	PermAudit            = "AUDIT"
)

// SystemPermissions lists all valid system permissions.
var SystemPermissions = []string{PermAdminister, PermCreateConnection, PermCreateUser, PermAudit}

// Password schemes.
const (
	SchemeBcrypt     = "bcrypt"
	SchemeGuacSHA256 = "guac_sha256"
)

// User is an application account.
type User struct {
	ID                 int64   `json:"id,string"`
	Username           string  `json:"username"`
	PasswordHash       string  `json:"-"`
	PasswordScheme     string  `json:"-"`
	PasswordSalt       string  `json:"-"`
	PasswordChangedAt  int64   `json:"passwordChangedAt"`
	MustChangePassword bool    `json:"mustChangePassword"`
	Disabled           bool    `json:"disabled"`
	ValidFrom          *string `json:"validFrom"`
	ValidUntil         *string `json:"validUntil"`
	AccessWindowStart  *string `json:"accessWindowStart"`
	AccessWindowEnd    *string `json:"accessWindowEnd"`
	Timezone           *string `json:"timezone"`
	FullName           *string `json:"fullName"`
	Email              *string `json:"email"`
	SecurityStamp      string  `json:"-"`
	CreatedAt          int64   `json:"createdAt"`
	UpdatedAt          int64   `json:"updatedAt"`
}

// UserGroup is a flat group of users.
type UserGroup struct {
	ID          int64  `json:"id,string"`
	Name        string `json:"name"`
	Disabled    bool   `json:"disabled"`
	CreatedAt   int64  `json:"createdAt"`
	MemberCount int    `json:"memberCount"`
}

// ConnectionGroup is a folder in the connection tree.
type ConnectionGroup struct {
	ID        int64  `json:"id,string"`
	ParentID  *int64 `json:"parentId,string"`
	Name      string `json:"name"`
	CreatedBy *int64 `json:"createdBy,string"`
	CreatedAt int64  `json:"createdAt"`
}

// Connection is a remote desktop/terminal target.
type Connection struct {
	ID                    int64             `json:"id,string"`
	ParentID              *int64            `json:"parentId,string"`
	Name                  string            `json:"name"`
	Protocol              string            `json:"protocol"`
	GuacdHost             *string           `json:"guacdHost"`
	GuacdPort             *int              `json:"guacdPort"`
	GuacdEncryption       *string           `json:"guacdEncryption"`
	MaxConnections        *int              `json:"maxConnections"`
	MaxConnectionsPerUser *int              `json:"maxConnectionsPerUser"`
	CreatedBy             *int64            `json:"createdBy,string"`
	CreatedAt             int64             `json:"createdAt"`
	UpdatedAt             int64             `json:"updatedAt"`
	Parameters            map[string]string `json:"-"`
}

// Session is a server-side login session.
type Session struct {
	ID            string
	UserID        int64
	CSRFToken     string
	SecurityStamp string
	CreatedAt     int64
	LastSeen      int64
	ExpiresAt     int64
	ClientIP      string
	UserAgent     string
}

// Preferences are per-user UI settings.
type Preferences struct {
	Theme            string `json:"theme"`
	Locale           string `json:"locale"`
	SidebarCollapsed bool   `json:"sidebarCollapsed"`
}

// HistoryEntry is one connection history row.
type HistoryEntry struct {
	ID             int64   `json:"id,string"`
	UserID         *int64  `json:"userId,string"`
	Username       string  `json:"username"`
	ConnectionID   *int64  `json:"connectionId,string"`
	ConnectionName string  `json:"connectionName"`
	Protocol       string  `json:"protocol"`
	ClientIP       string  `json:"clientIp"`
	TunnelID       string  `json:"tunnelId"`
	StartAt        int64   `json:"startAt"`
	EndAt          *int64  `json:"endAt"`
	CloseReason    *string `json:"closeReason"`
	Recording      string  `json:"-"`
	Typescript     string  `json:"-"`
	HasRecording   bool    `json:"hasRecording"`
	HasTypescript  bool    `json:"hasTypescript"`
}

// AuditEvent is one audit log row.
type AuditEvent struct {
	ID         int64          `json:"id,string"`
	At         int64          `json:"at"`
	ActorID    *int64         `json:"actorId,string"`
	ActorName  string         `json:"actorName"`
	ClientIP   string         `json:"clientIp"`
	Action     string         `json:"action"`
	TargetType string         `json:"targetType"`
	TargetID   string         `json:"targetId"`
	TargetName string         `json:"targetName"`
	Outcome    string         `json:"outcome"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// Grants are the permission grants of one subject.
type Grants struct {
	System      []string `json:"system"`
	Connections []int64  `json:"connections"`
	Groups      []int64  `json:"connectionGroups"`
}

// Page bounds list queries.
type Page struct {
	Limit  int
	Offset int
}

// Normalize applies defaults and caps.
func (p Page) Normalize() Page {
	if p.Limit <= 0 || p.Limit > 500 {
		p.Limit = 50
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}
