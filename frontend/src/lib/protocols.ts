// Data-driven connection forms: fields per protocol, grouped in sections.
// Secret fields mirror internal/secrets (server side).
import type { Protocol } from '../api/types'

export type FieldType = 'text' | 'number' | 'password' | 'textarea' | 'bool' | 'select'

export interface Field {
  name: string
  type: FieldType
  section: 'network' | 'authentication' | 'display' | 'behavior' | 'sftp' | 'advanced'
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
]

const clipboard: Field[] = [
  { name: 'disable-copy', type: 'bool', section: 'behavior' },
  { name: 'disable-paste', type: 'bool', section: 'behavior' },
]

const recording: Field[] = [
  { name: 'recording-path', type: 'text', section: 'advanced', placeholder: '/record' },
  {
    name: 'recording-name',
    type: 'text',
    section: 'advanced',
    placeholder: '${GUAC_USERNAME}-${GUAC_DATE}-${GUAC_TIME}',
  },
  { name: 'create-recording-path', type: 'bool', section: 'advanced' },
]

export const PROTOCOL_FIELDS: Record<Protocol, Field[]> = {
  ssh: [
    { name: 'hostname', type: 'text', section: 'network', required: true },
    { name: 'port', type: 'number', section: 'network', placeholder: '22' },
    { name: 'host-key', type: 'text', section: 'network' },
    { name: 'username', type: 'text', section: 'authentication' },
    { name: 'password', type: 'password', section: 'authentication', secret: true },
    { name: 'private-key', type: 'textarea', section: 'authentication', secret: true },
    { name: 'passphrase', type: 'password', section: 'authentication', secret: true },
    { name: 'font-name', type: 'text', section: 'display', placeholder: 'monospace' },
    { name: 'font-size', type: 'number', section: 'display', placeholder: '12' },
    {
      name: 'color-scheme',
      type: 'select',
      section: 'display',
      options: ['', 'black-white', 'gray-black', 'green-black', 'white-black'],
    },
    { name: 'scrollback', type: 'number', section: 'display', placeholder: '1000' },
    { name: 'command', type: 'text', section: 'behavior' },
    { name: 'locale', type: 'text', section: 'behavior', placeholder: 'en_US.UTF-8' },
    { name: 'timezone', type: 'text', section: 'behavior', placeholder: 'America/Sao_Paulo' },
    { name: 'server-alive-interval', type: 'number', section: 'behavior' },
    { name: 'read-only', type: 'bool', section: 'behavior' },
    ...clipboard,
    ...sftp,
    { name: 'typescript-path', type: 'text', section: 'advanced' },
    { name: 'typescript-name', type: 'text', section: 'advanced' },
    { name: 'create-typescript-path', type: 'bool', section: 'advanced' },
    ...recording,
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
    { name: 'timezone', type: 'text', section: 'behavior', placeholder: 'America/Sao_Paulo' },
    { name: 'console', type: 'bool', section: 'behavior' },
    { name: 'initial-program', type: 'text', section: 'behavior' },
    { name: 'enable-printing', type: 'bool', section: 'behavior' },
    { name: 'enable-drive', type: 'bool', section: 'behavior' },
    { name: 'drive-name', type: 'text', section: 'behavior' },
    { name: 'drive-path', type: 'text', section: 'behavior', placeholder: '/drive' },
    { name: 'create-drive-path', type: 'bool', section: 'behavior' },
    { name: 'enable-audio-input', type: 'bool', section: 'behavior' },
    ...clipboard,
    { name: 'gateway-hostname', type: 'text', section: 'advanced' },
    { name: 'gateway-port', type: 'number', section: 'advanced' },
    { name: 'gateway-username', type: 'text', section: 'advanced' },
    { name: 'gateway-password', type: 'password', section: 'advanced', secret: true },
    { name: 'gateway-domain', type: 'text', section: 'advanced' },
    ...sftp,
    ...recording,
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
    ...recording,
  ],
}

export const SECTIONS = ['network', 'authentication', 'display', 'behavior', 'sftp', 'advanced'] as const

export const PROTOCOLS: Protocol[] = ['ssh', 'rdp', 'vnc']
