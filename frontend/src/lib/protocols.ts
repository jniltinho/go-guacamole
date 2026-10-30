// Data-driven connection forms: fields per protocol, grouped in sections.
// Secret fields mirror internal/secrets (server side).
import type { Protocol } from '../api/types'

export type FieldType = 'text' | 'number' | 'password' | 'textarea' | 'bool' | 'select'

export interface Field {
  name: string
  type: FieldType
  section: (typeof SECTIONS)[number]
  options?: string[]
  placeholder?: string
  required?: boolean
  secret?: boolean
}

const sftp: Field[] = [
  { name: 'enable-sftp', type: 'bool', section: 'sftp' },
  { name: 'sftp-hostname', type: 'text', section: 'sftp' },
  { name: 'sftp-port', type: 'number', section: 'sftp', placeholder: '22' },
  { name: 'sftp-username', type: 'text', section: 'sftp' },
  { name: 'sftp-password', type: 'password', section: 'sftp', secret: true },
  { name: 'sftp-private-key', type: 'textarea', section: 'sftp', secret: true },
  { name: 'sftp-passphrase', type: 'password', section: 'sftp', secret: true },
  { name: 'sftp-root-directory', type: 'text', section: 'sftp', placeholder: '/' },
  { name: 'sftp-directory', type: 'text', section: 'sftp' },
  { name: 'sftp-disable-download', type: 'bool', section: 'sftp' },
  { name: 'sftp-disable-upload', type: 'bool', section: 'sftp' },
]

const clipboard: Field[] = [
  { name: 'disable-copy', type: 'bool', section: 'clipboard' },
  { name: 'disable-paste', type: 'bool', section: 'clipboard' },
]

// IANA zones known to the browser; guacd takes the same names.
const timezones = ['', ...Intl.supportedValuesOf('timeZone')]

// Recording is set by the recording policy (Recording panel), never by connection parameters.

export const PROTOCOL_FIELDS: Record<Protocol, Field[]> = {
  ssh: [
    { name: 'hostname', type: 'text', section: 'network', required: true },
    { name: 'port', type: 'number', section: 'network', placeholder: '22' },
    { name: 'host-key', type: 'text', section: 'network' },
    { name: 'username', type: 'text', section: 'authentication' },
    { name: 'password', type: 'password', section: 'authentication', secret: true },
    { name: 'private-key', type: 'textarea', section: 'authentication', secret: true },
    { name: 'passphrase', type: 'password', section: 'authentication', secret: true },
    {
      name: 'color-scheme',
      type: 'select',
      section: 'display',
      // '' = the user's terminal preference; named palettes are expanded by the server
      options: [
        '', 'gogu-dark', 'gogu-light', 'noc', 'bionexo', 'dracula', 'nord', 'one-dark', 'monokai',
        'solarized-dark', 'solarized-light', 'gruvbox-dark', 'gruvbox-light', 'tango-dark', 'tango-light',
        'campbell', 'ubuntu', 'breeze', 'xterm', 'matrix', 'high-contrast',
        'black-white', 'gray-black', 'green-black', 'white-black',
      ], // prettier-ignore
    },
    { name: 'font-name', type: 'text', section: 'display', placeholder: 'monospace' },
    {
      name: 'font-size',
      type: 'select',
      section: 'display',
      options: ['', '8', '9', '10', '11', '12', '14', '18', '24', '30', '36', '48', '60', '72', '96'],
    },
    { name: 'scrollback', type: 'number', section: 'display', placeholder: '1000' },
    { name: 'read-only', type: 'bool', section: 'display' },
    ...clipboard,
    { name: 'command', type: 'text', section: 'session' },
    { name: 'locale', type: 'text', section: 'session', placeholder: 'en_US.UTF-8' },
    { name: 'timezone', type: 'select', section: 'session', options: timezones },
    { name: 'server-alive-interval', type: 'number', section: 'session' },
    { name: 'backspace', type: 'select', section: 'terminal', options: ['', '127', '8'] },
    {
      name: 'terminal-type',
      type: 'select',
      section: 'terminal',
      options: ['', 'ansi', 'linux', 'vt100', 'vt220', 'xterm', 'xterm-256color'],
    },
    ...sftp,
  ],
  rdp: [
    { name: 'hostname', type: 'text', section: 'network', required: true },
    { name: 'port', type: 'number', section: 'network', placeholder: '3389' },
    { name: 'username', type: 'text', section: 'authentication' },
    { name: 'password', type: 'password', section: 'authentication', secret: true },
    { name: 'domain', type: 'text', section: 'authentication' },
    {
      name: 'security',
      type: 'select',
      section: 'authentication',
      options: ['', 'any', 'nla', 'nla-ext', 'tls', 'vmconnect', 'rdp'],
    },
    { name: 'ignore-cert', type: 'bool', section: 'authentication' },
    { name: 'color-depth', type: 'select', section: 'display', options: ['', '8', '16', '24', '32'] },
    {
      name: 'resize-method',
      type: 'select',
      section: 'display',
      options: ['', 'display-update', 'reconnect'],
    },
    { name: 'enable-wallpaper', type: 'bool', section: 'display' },
    { name: 'enable-font-smoothing', type: 'bool', section: 'display' },
    {
      name: 'server-layout',
      type: 'select',
      section: 'behavior',
      options: [
        '',
        'en-us-qwerty',
        'pt-br-qwerty',
        'en-gb-qwerty',
        'es-es-qwerty',
        'de-de-qwertz',
        'fr-fr-azerty',
        'failsafe',
      ],
    },
    { name: 'timezone', type: 'select', section: 'behavior', options: timezones },
    { name: 'console', type: 'bool', section: 'behavior' },
    { name: 'initial-program', type: 'text', section: 'behavior' },
    { name: 'enable-printing', type: 'bool', section: 'behavior' },
    { name: 'enable-drive', type: 'bool', section: 'behavior' },
    { name: 'drive-name', type: 'text', section: 'behavior' },
    { name: 'drive-path', type: 'text', section: 'behavior', placeholder: '/drive' },
    { name: 'create-drive-path', type: 'bool', section: 'behavior' },
    { name: 'enable-audio-input', type: 'bool', section: 'behavior' },
    { name: 'disable-download', type: 'bool', section: 'behavior' },
    { name: 'disable-upload', type: 'bool', section: 'behavior' },
    ...clipboard,
    { name: 'gateway-hostname', type: 'text', section: 'advanced' },
    { name: 'gateway-port', type: 'number', section: 'advanced' },
    { name: 'gateway-username', type: 'text', section: 'advanced' },
    { name: 'gateway-password', type: 'password', section: 'advanced', secret: true },
    { name: 'gateway-domain', type: 'text', section: 'advanced' },
    ...sftp,
  ],
  vnc: [
    { name: 'hostname', type: 'text', section: 'network', required: true },
    { name: 'port', type: 'number', section: 'network', placeholder: '5900' },
    { name: 'dest-host', type: 'text', section: 'network' },
    { name: 'dest-port', type: 'number', section: 'network' },
    { name: 'username', type: 'text', section: 'authentication' },
    { name: 'password', type: 'password', section: 'authentication', secret: true },
    { name: 'color-depth', type: 'select', section: 'display', options: ['', '8', '16', '24', '32'] },
    { name: 'cursor', type: 'select', section: 'display', options: ['', 'local', 'remote'] },
    { name: 'swap-red-blue', type: 'bool', section: 'display' },
    { name: 'read-only', type: 'bool', section: 'behavior' },
    ...clipboard,
    ...sftp,
  ],
}

export const SECTIONS = [
  'network',
  'authentication',
  'display',
  'clipboard',
  'session',
  'terminal',
  'behavior',
  'sftp',
  'advanced',
] as const

export const PROTOCOLS: Protocol[] = ['ssh', 'rdp', 'vnc']
