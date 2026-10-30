export type Protocol = 'ssh' | 'rdp' | 'vnc'

export interface User {
  id: string
  username: string
  fullName: string | null
  email: string | null
  disabled: boolean
  mustChangePassword: boolean
  validFrom: string | null
  validUntil: string | null
  accessWindowStart: string | null
  accessWindowEnd: string | null
  timezone: string | null
  passwordChangedAt: number
  authSource: 'local' | 'ldap' | 'oidc' | 'saml' | 'header' | 'json'
  totpEnrolled: boolean
  createdAt: number
  updatedAt: number
  groups?: string[]
}

export interface Preferences {
  theme: string
  locale: string
  sidebarCollapsed: boolean
  effects?: '' | 'full' | 'reduced' | 'off'
  terminalScheme?: string
}

export interface Me {
  user: User
  permissions: string[]
  isAdmin: boolean
  csrfToken: string
  preferences: Preferences
  expiresAt: number
  groups: string[]
  build: { version: string; gitCommit: string; buildDate: string }
  recording: boolean
  /** the job dashboard (/asynqmon) is reachable: administrator and queue up */
  jobsDashboard?: boolean
}

export interface Source {
  via: 'admin' | 'direct' | 'group'
  group?: string
  folder?: string
}

export interface Connection {
  id: string
  parentId: string | null
  name: string
  protocol: Protocol
  hostname: string
  port: string
  groupPath: string[]
  favorite: boolean
  status: 'online' | 'offline' | 'unknown'
  activeSessions: number
  canEdit: boolean
  guacdHost: string | null
  guacdPort: number | null
  guacdEncryption: string | null
  maxConnections: number | null
  maxConnectionsPerUser: number | null
  parameters?: Record<string, string | { set: boolean }>
  sources?: Source[]
  recording?: ConnectionRecording
}

export interface ConnectionGroup {
  id: string
  name: string
  parentId: string | null
  canEdit: boolean
  inheritable?: boolean
  groups?: ConnectionGroup[]
  connections?: Connection[]
}

export interface TunnelInfo {
  id: string
  userId: string
  username: string
  connectionId: string
  connectionName: string
  protocol: Protocol
  clientIp: string
  startAt: number
  recVideo?: boolean
  recText?: boolean
  recKeys?: boolean
}

export interface RecordingSnapshot {
  video: boolean
  text: boolean
  keys: boolean
  exception: boolean
}

export type Tri = 'inherit' | 'on' | 'off'

/** Per-connection recording policy (connection detail, AUDIT/ADMINISTER edit). */
export interface ConnectionRecording {
  video: Tri
  text: Tri
  keys: Tri
  retentionVideoDays: number | null
  retentionTextDays: number | null
  version: number
  effective: RecordingSnapshot
  effectiveVideoDays: number
  effectiveTextDays: number
  canEdit: boolean
  defaults: RecordingDefaults
}

export interface RecordingDefaults {
  saved?: boolean
  video: boolean
  text: boolean
  keys: boolean
  retentionVideoDays: number
  retentionTextDays: number
  hideNotice?: boolean
}

export interface AuditEvent {
  id: string
  at: number
  actorId: string | null
  actorName: string
  clientIp: string
  action: string
  targetType: string
  targetId: string
  targetName: string
  outcome: string
  metadata?: Record<string, unknown>
}

export interface HistoryEntry {
  id: string
  userId: string | null
  username: string
  connectionId: string | null
  connectionName: string
  protocol: Protocol
  clientIp: string
  tunnelId: string
  startAt: number
  endAt: number | null
  closeReason: string | null
  hasRecording: boolean
  hasTypescript: boolean
  recVideo: boolean | null
  recText: boolean | null
  recKeys: boolean | null
  recException: boolean
  videoCapture: string
  textCapture: string
  videoFiles: string
  textFiles: string
  videoPurgedAt: number | null
  textPurgedAt: number | null
  videoStorage: string
  textStorage: string
  videoStorageError?: string
  textStorageError?: string
  holdReason?: string
  holdBy?: string
  holdAt: number | null
}

export interface UserGroup {
  id: string
  name: string
  disabled: boolean
  createdAt: number
  memberCount: number
}

export interface PermissionCount {
  type: 'user' | 'group'
  id: string
  name: string
  permissions: number
  inheritedFrom: string[]
}

export interface Dashboard {
  counts: { connections: number; activeSessions: number; users?: number; groups?: number }
  connections: Connection[]
  recentActivity: AuditEvent[]
  activeSessions: TunnelInfo[]
  permissionOverview?: PermissionCount[]
}

export interface List<T> {
  items: T[]
  total: number
}

/** Direct grants as returned by the API (ids are numbers there). */
export interface Grants {
  system: string[]
  connections: Array<string | number>
  connectionGroups: Array<string | number>
}
