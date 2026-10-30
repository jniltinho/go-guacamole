<script setup lang="ts">
import {
  ArrowLeft,
  Clipboard,
  FolderOpen,
  Keyboard,
  LogOut,
  Maximize,
  Minimize,
  PanelRightClose,
  RotateCw,
} from '@lucide/vue'
import Guacamole from 'guacamole-common-js'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { api, ApiError } from '../api/client'
import type { RecordingSnapshot } from '../api/types'
import FileBrowser from '../components/FileBrowser.vue'
import { useTheme } from '../composables/useTheme'
import { basename, joinPath, receive, send, type Transfer } from '../lib/transfer'

interface LaunchResult {
  ticket: string
  tunnelId: string
  protocol: string
  connectionName: string
  disableCopy: boolean
  disablePaste: boolean
  recording: RecordingSnapshot
  /** false when the administrator hides the recording notice */
  recordingNotice?: boolean
}

const route = useRoute()
const router = useRouter()
const { t, te } = useI18n()
const { setInClient } = useTheme()

const viewport = ref<HTMLDivElement>()
const host = ref<HTMLDivElement>()
const state = ref<'connecting' | 'waiting' | 'connected' | 'disconnected' | 'error'>('connecting')
const errorText = ref('')
// recording notice from the launch snapshot (what this session records)
const recording = ref<RecordingSnapshot | null>(null)
const recKinds = computed(() => {
  const r = recording.value
  if (!r || r.exception) return ''
  return (['video', 'text', 'keys'] as const)
    .filter((k) => r[k])
    .map((k) => t('recordingPolicy.kinds.' + k))
    .join(' · ')
})
const name = ref('')
const menuOpen = ref(false)
const clipboard = ref('')
const fullscreen = ref(false)
// filesystems exposed by guacd (SFTP / RDP drive) and the transfers of this session
const filesystems = shallowRef<Array<{ object: Guacamole.Object; name: string }>>([])
const transfers = ref<Transfer[]>([])
const dragging = ref(false)
const protocol = ref('')
const disableCopy = ref(false)
const disablePaste = ref(false)
// last text guacd sent (the terminal selection in SSH); `clipboard` is the editable buffer
const remoteText = ref('')
// right-click Copy/Paste menu of SSH sessions, at viewport-relative coordinates
const ctxMenu = ref<{ x: number; y: number } | null>(null)
const ctxEl = ref<HTMLDivElement>()

let client: Guacamole.Client | null = null
let lastMouse = new Guacamole.Mouse.State(0, 0, false, false, false, false, false)
let keyboard: Guacamole.Keyboard | null = null
let resizeTimer = 0
let observer: ResizeObserver | null = null

const SUPPORTED_IMAGES = ['image/png', 'image/jpeg', 'image/webp']

const overlay = computed(() => state.value !== 'connected')

function statusMessage(code: number): string {
  const k = 'guac.' + code
  return te(k) ? t(k) : t('guac.default', { code })
}

function rescale() {
  if (!client || !viewport.value) return
  const display = client.getDisplay()
  const w = display.getWidth()
  const h = display.getHeight()
  if (!w || !h) return
  const box = viewport.value.getBoundingClientRect()
  display.scale(Math.min(box.width / w, box.height / h))
}

function sendSize() {
  if (!client || !viewport.value) return
  const box = viewport.value.getBoundingClientRect()
  const ratio = window.devicePixelRatio || 1
  client.sendSize(Math.floor(box.width * ratio), Math.floor(box.height * ratio))
}

function queryString(ticket: string): string {
  const box = viewport.value!.getBoundingClientRect()
  const ratio = window.devicePixelRatio || 1
  const q = new URLSearchParams()
  q.set('ticket', ticket)
  q.set('width', String(Math.floor(box.width * ratio)))
  q.set('height', String(Math.floor(box.height * ratio)))
  q.set('dpi', String(Math.floor(96 * ratio)))
  q.set('timezone', Intl.DateTimeFormat().resolvedOptions().timeZone)
  for (const a of Guacamole.AudioPlayer.getSupportedTypes()) q.append('audio', a)
  for (const v of Guacamole.VideoPlayer.getSupportedTypes()) q.append('video', v)
  for (const i of SUPPORTED_IMAGES) q.append('image', i)
  return q.toString()
}

async function start() {
  teardown()
  state.value = 'connecting'
  errorText.value = ''
  let launch: LaunchResult
  try {
    launch = await api.post<LaunchResult>('/sessions', {
      connectionId: route.params.connectionId,
      // the theme in use: "follow" terminals match it even before any preference is saved
      uiTheme: document.documentElement.dataset.theme,
    })
  } catch (e) {
    state.value = 'error'
    errorText.value =
      e instanceof ApiError && te('errors.' + e.code) ? t('errors.' + e.code) : t('common.error')
    return
  }
  name.value = launch.connectionName
  protocol.value = launch.protocol
  disableCopy.value = launch.disableCopy
  disablePaste.value = launch.disablePaste
  recording.value = launch.recordingNotice === false ? null : (launch.recording ?? null)
  remoteText.value = ''
  document.title = `${launch.connectionName} · ${t('app.name')}`

  const scheme = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const tunnel = new Guacamole.WebSocketTunnel(`${scheme}//${location.host}/api/v1/tunnel`)
  client = new Guacamole.Client(tunnel)
  const display = client.getDisplay()
  const el = display.getElement()
  host.value!.replaceChildren(el)

  display.onresize = () => rescale()
  display.oncursor = (canvas: HTMLCanvasElement, x: number, y: number) => {
    el.style.cursor = `url(${canvas.toDataURL('image/png')}) ${x} ${y}, auto`
  }

  client.onstatechange = (s: number) => {
    if (s === 1) state.value = 'connecting'
    else if (s === 2) state.value = 'waiting'
    else if (s === 3) {
      state.value = 'connected'
      nextTick(() => {
        rescale()
        viewport.value?.focus()
      })
    } else if (s === 5 && state.value !== 'error') state.value = 'disconnected'
  }
  const onError = (status: Guacamole.Status) => {
    state.value = 'error'
    errorText.value =
      status.message === 'RECORDING_FAILED' ? t('errors.RECORDING_FAILED') : statusMessage(status.code)
  }
  client.onerror = onError
  tunnel.onerror = onError
  client.onfilesystem = (object: Guacamole.Object, fsName: string) => {
    filesystems.value = [...filesystems.value, { object, name: fsName }]
  }
  // files pushed by the server (RDP "Download" folder, guacctl in SSH)
  client.onfile = (stream: Guacamole.InputStream, mimetype: string, filename: string) => {
    receive(stream, mimetype, track(filename, false, 0))
  }
  client.onclipboard = (stream: Guacamole.InputStream, mimetype: string) => {
    if (!mimetype.startsWith('text/')) return
    const reader = new Guacamole.StringReader(stream)
    let data = ''
    reader.ontext = (text: string) => (data += text)
    reader.onend = () => {
      clipboard.value = remoteText.value = data
      navigator.clipboard?.writeText(data).catch(() => {})
    }
  }

  // input
  const mouse = new Guacamole.Mouse(el)
  mouse.onEach(['mousedown', 'mouseup', 'mousemove'], (e) => {
    const s = (e as Guacamole.Mouse.Event).state
    // in SSH the right button opens our menu instead of guacd's paste-on-right-click
    lastMouse =
      protocol.value === 'ssh'
        ? new Guacamole.Mouse.State(s.x, s.y, s.left, s.middle, false, s.up, s.down)
        : s
    client?.sendMouseState(lastMouse, true)
  })
  const touch = new Guacamole.Mouse.Touchscreen(el)
  touch.onEach(['mousedown', 'mouseup', 'mousemove'], (e) => {
    client?.sendMouseState((e as Guacamole.Mouse.Event).state, true)
  })

  keyboard = new Guacamole.Keyboard(viewport.value!)
  const pressed = new Set<number>()
  // keys whose keydown reached guacd; their keyup always follows, even after a menu opened
  const forwarded = new Set<number>()
  keyboard.onkeydown = (keysym: number) => {
    pressed.add(keysym)
    // Ctrl+Alt+Shift toggles the session menu
    const ctrl = pressed.has(0xffe3) || pressed.has(0xffe4)
    const alt = pressed.has(0xffe9) || pressed.has(0xffea)
    const shift = pressed.has(0xffe1) || pressed.has(0xffe2)
    if (ctrl && alt && shift) {
      menuOpen.value = !menuOpen.value
      keyboard?.reset()
      pressed.clear()
      return false
    }
    // while the context menu is open the keyboard drives it (Tab, Enter), not the shell
    if (ctxMenu.value) {
      if (keysym !== 0xff1b) return true
      closeCtxMenu()
      return false
    }
    if (menuOpen.value) return true
    // terminal-style Ctrl+Shift+C/V and Ctrl+Insert / Shift+Insert, never sent to the remote shell
    const insert = keysym === 0xff63
    const copyKey = (ctrl && shift && (keysym === 0x43 || keysym === 0x63)) || (ctrl && !shift && insert)
    const pasteKey = (ctrl && shift && (keysym === 0x56 || keysym === 0x76)) || (shift && !ctrl && insert)
    if (protocol.value === 'ssh' && !alt && (copyKey || pasteKey)) {
      void (copyKey ? copyRemote() : pasteLocal())
      return false
    }
    forwarded.add(keysym)
    client?.sendKeyEvent(1, keysym)
    return false
  }
  keyboard.onkeyup = (keysym: number) => {
    pressed.delete(keysym)
    if (forwarded.delete(keysym)) client?.sendKeyEvent(0, keysym)
  }

  client.connect(queryString(launch.ticket))
}

function track(fileName: string, upload: boolean, total: number): Transfer {
  const t = reactive({ name: fileName, upload, bytes: 0, total, done: false, error: '' })
  transfers.value = [t, ...transfers.value].slice(0, 20)
  return t
}

function download(object: Guacamole.Object, path: string) {
  object.requestInputStream(path, (stream, mimetype) =>
    receive(stream, mimetype, track(basename(path), false, 0)),
  )
}

// Without a target folder the file goes where guacd puts uploads (drive root / sftp-directory).
async function upload(files: Iterable<File>, object?: Guacamole.Object, dir?: string, done?: () => void) {
  if (!client) return
  for (const f of files) {
    const stream =
      object && dir
        ? object.createOutputStream(f.type || 'application/octet-stream', joinPath(dir, f.name))
        : client.createFileStream(f.type || 'application/octet-stream', f.name)
    await send(stream, f, track(f.name, true, f.size))
  }
  done?.()
}

function onDrop(e: DragEvent) {
  dragging.value = false
  if (state.value === 'connected' && e.dataTransfer?.files.length) upload(Array.from(e.dataTransfer.files))
}

function pct(t: Transfer) {
  return t.total ? Math.min(100, Math.round((t.bytes / t.total) * 100)) : 0
}

function teardown() {
  if (keyboard) {
    keyboard.onkeydown = null
    keyboard.onkeyup = null
    keyboard.reset()
    keyboard = null
  }
  if (client) {
    client.onstatechange = null
    client.onerror = null
    client.onfilesystem = null
    client.onfile = null
    client.disconnect()
    client = null
  }
  host.value?.replaceChildren()
  filesystems.value = []
  ctxMenu.value = null
}

function sendClipboard() {
  if (!client) return
  const stream = client.createClipboardStream('text/plain')
  const writer = new Guacamole.StringWriter(stream)
  writer.sendText(clipboard.value)
  writer.sendEnd()
}

// Capture phase: Guacamole.Mouse swallows contextmenu on the display element.
async function onViewportMouseDown(e: MouseEvent) {
  if (e.button !== 2 || protocol.value !== 'ssh' || state.value !== 'connected') return
  if (!host.value?.contains(e.target as Node)) return
  e.preventDefault() // otherwise the browser refocuses the viewport after we focus the menu
  const box = viewport.value!.getBoundingClientRect()
  const x = e.clientX - box.left
  const y = e.clientY - box.top
  ctxMenu.value = { x, y }
  await nextTick()
  // keep the menu inside the viewport, then hand it the keyboard
  const m = ctxEl.value!.getBoundingClientRect()
  ctxMenu.value = { x: Math.min(x, box.width - m.width), y: Math.min(y, box.height - m.height) }
  ctxEl.value!.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus()
}

function closeCtxMenu() {
  ctxMenu.value = null
  viewport.value?.focus()
}

// The terminal selection reaches us through onclipboard; this puts it on the local clipboard.
async function copyRemote() {
  closeCtxMenu()
  if (disableCopy.value || !remoteText.value) return
  try {
    await navigator.clipboard.writeText(remoteText.value)
  } catch {
    clipboard.value = remoteText.value
    menuOpen.value = true
  }
}

async function pasteLocal() {
  closeCtxMenu()
  // with disable-paste guacd drops our text, and the click below would paste its stale selection
  if (disablePaste.value) return
  try {
    clipboard.value = await navigator.clipboard.readText()
  } catch {
    // no permission or insecure context: the side menu textarea is the fallback
    menuOpen.value = true
    return
  }
  sendClipboard()
  // guacd's terminal types its clipboard on a middle-button release
  const { x, y } = lastMouse
  client?.sendMouseState(new Guacamole.Mouse.State(x, y, false, true, false, false, false), true)
  client?.sendMouseState(new Guacamole.Mouse.State(x, y, false, false, false, false, false), true)
}

function ctrlAltDel() {
  if (!client) return
  for (const k of [0xffe3, 0xffe9, 0xffff]) client.sendKeyEvent(1, k)
  for (const k of [0xffff, 0xffe9, 0xffe3]) client.sendKeyEvent(0, k)
}

async function toggleFullscreen() {
  if (document.fullscreenElement) await document.exitFullscreen()
  else await document.documentElement.requestFullscreen()
}

function onFullscreen() {
  fullscreen.value = !!document.fullscreenElement
}

function disconnect() {
  teardown()
  router.push('/connections')
}

function onWindowResize() {
  rescale()
  clearTimeout(resizeTimer)
  resizeTimer = window.setTimeout(sendSize, 300)
}

onMounted(() => {
  setInClient(true)
  window.addEventListener('resize', onWindowResize)
  document.addEventListener('fullscreenchange', onFullscreen)
  observer = new ResizeObserver(onWindowResize)
  observer.observe(viewport.value!)
  start()
})

onBeforeUnmount(() => {
  setInClient(false)
  window.removeEventListener('resize', onWindowResize)
  document.removeEventListener('fullscreenchange', onFullscreen)
  observer?.disconnect()
  clearTimeout(resizeTimer)
  teardown()
})
</script>

<template>
  <div class="client-view fixed inset-0 flex bg-black text-white" data-testid="client-view">
    <div
      ref="viewport"
      class="relative min-w-0 flex-1 overflow-hidden outline-none"
      tabindex="0"
      data-testid="client-viewport"
      @contextmenu.prevent
      @mousedown.capture="onViewportMouseDown"
      @dragover.prevent="dragging = state === 'connected'"
      @dragleave="dragging = false"
      @drop.prevent="onDrop"
    >
      <div
        v-if="dragging"
        class="pointer-events-none absolute inset-0 z-20 flex items-center justify-center border-4 border-dashed border-white/60 bg-black/60 text-lg font-semibold"
      >
        {{ t('files.drop') }}
      </div>
      <div ref="host" class="absolute inset-0 flex items-center justify-center" />
      <template v-if="ctxMenu">
        <div class="absolute inset-0 z-30" @mousedown="closeCtxMenu" />
        <div
          ref="ctxEl"
          class="absolute z-40 grid border border-line bg-surface py-1 text-sm text-fg shadow-md"
          :style="{ left: ctxMenu.x + 'px', top: ctxMenu.y + 'px' }"
          role="menu"
          data-testid="ssh-context-menu"
        >
          <button
            v-for="item in [
              {
                label: t('client.copy'),
                keys: 'Ctrl+Shift+C · Ctrl+Ins',
                off: disableCopy || !remoteText,
                run: copyRemote,
              },
              {
                label: t('client.paste'),
                keys: 'Ctrl+Shift+V · Shift+Ins',
                off: disablePaste,
                run: pasteLocal,
              },
            ]"
            :key="item.label"
            class="flex items-center justify-between gap-6 px-3 py-1 text-left whitespace-nowrap outline-none hover:bg-surface-2 focus-visible:bg-surface-2 disabled:pointer-events-none disabled:opacity-40"
            role="menuitem"
            :disabled="item.off"
            @click="item.run"
          >
            {{ item.label }}<span class="text-xs text-muted">{{ item.keys }}</span>
          </button>
        </div>
      </template>
      <div v-if="overlay" class="absolute inset-0 z-10 flex items-center justify-center bg-black/80 p-4">
        <div class="w-full max-w-md border border-white/20 bg-neutral-900 p-6 text-center" role="status">
          <template v-if="state === 'connecting' || state === 'waiting'">
            <RotateCw class="mx-auto mb-3 h-8 w-8 animate-spin" />
            <p class="font-semibold">{{ t('client.connecting', { name: name || '…' }) }}</p>
            <p v-if="state === 'waiting'" class="mt-1 text-sm text-white/60">{{ t('client.waiting') }}</p>
            <p v-if="recKinds" class="mt-3 text-sm text-red-300" data-testid="rec-notice">
              {{ t('recordingPolicy.notice', { kinds: recKinds }) }}
            </p>
            <p v-else-if="recording?.exception" class="mt-3 text-sm text-amber-300">
              {{ t('recordingPolicy.exceptionNotice') }}
            </p>
          </template>
          <template v-else>
            <p class="mb-1 text-lg font-semibold">{{ t('client.disconnected') }}</p>
            <p v-if="errorText" class="mb-4 text-sm text-white/75" data-testid="client-error">
              {{ errorText }}
            </p>
            <div class="flex justify-center gap-2">
              <button class="btn btn-primary" data-testid="reconnect" @click="start">
                <RotateCw class="h-4 w-4" /> {{ t('client.reconnect') }}
              </button>
              <RouterLink to="/connections" class="btn"
                ><ArrowLeft class="h-4 w-4" /> {{ t('client.backHome') }}</RouterLink
              >
            </div>
          </template>
        </div>
      </div>
      <button
        v-if="!menuOpen"
        class="absolute top-2 right-2 z-20 bg-white/10 px-2 py-1 text-xs opacity-40 hover:opacity-100"
        :title="t('client.menuHint')"
        data-testid="open-menu"
        @click="menuOpen = true"
      >
        {{ t('client.menu') }}
      </button>
    </div>

    <!-- not an <aside>: theme rules for the app sidebar must not repaint the session menu -->
    <div
      v-if="menuOpen"
      role="complementary"
      :aria-label="t('client.menu')"
      class="z-30 flex w-80 shrink-0 flex-col gap-4 overflow-y-auto border-l border-white/20 bg-neutral-900 p-4"
      data-testid="client-menu"
    >
      <div class="flex items-center justify-between">
        <h2 class="font-semibold">{{ name }}</h2>
        <button
          class="opacity-70 hover:opacity-100"
          :aria-label="t('common.close')"
          @click="menuOpen = false"
        >
          <PanelRightClose class="h-5 w-5" />
        </button>
      </div>
      <!-- what this session records, discreetly under the title (announced before connecting too) -->
      <p
        v-if="recKinds && state === 'connected'"
        class="-mt-2 flex items-center gap-1.5 text-[11px] text-red-300/90"
        :title="t('recordingPolicy.notice', { kinds: recKinds })"
        data-testid="rec-badge"
      >
        <span class="h-1.5 w-1.5 shrink-0 animate-pulse bg-red-500" />REC · {{ recKinds }}
      </p>
      <p class="text-xs text-white/50">{{ t('client.menuHint') }}</p>
      <div>
        <label class="mb-1 flex items-center gap-2 text-sm font-semibold"
          ><Clipboard class="h-4 w-4" /> {{ t('client.clipboard') }}</label
        >
        <p class="mb-2 text-xs text-white/50">{{ t('client.clipboardHint') }}</p>
        <p v-if="protocol === 'ssh'" class="mb-2 text-xs text-white/50" data-testid="ssh-clipboard-hint">
          {{ t('client.sshHint') }}
        </p>
        <textarea
          v-model="clipboard"
          rows="6"
          class="w-full border border-white/20 bg-black p-2 font-mono text-sm text-white"
          data-testid="clipboard"
        />
        <button
          class="mt-2 border border-white/30 px-3 py-1.5 text-sm hover:bg-white/10"
          @click="sendClipboard"
        >
          {{ t('client.sendClipboard') }}
        </button>
      </div>
      <div v-if="filesystems.length || transfers.length">
        <label class="mb-1 flex items-center gap-2 text-sm font-semibold"
          ><FolderOpen class="h-4 w-4" /> {{ t('files.title') }}</label
        >
        <p class="mb-2 text-xs text-white/50">{{ t('files.hint') }}</p>
        <div class="grid gap-2">
          <FileBrowser
            v-for="fs in filesystems"
            :key="fs.name"
            :object="fs.object"
            :name="fs.name"
            @download="(p) => download(fs.object, p)"
            @upload="(files, dir, done) => upload(files, fs.object, dir, done)"
          />
        </div>
        <ul v-if="transfers.length" class="mt-2 grid gap-1 text-xs" data-testid="transfers">
          <li v-for="(x, i) in transfers" :key="i" class="border border-white/10 p-1.5">
            <div class="flex justify-between gap-2">
              <span class="truncate">{{ x.upload ? '↑' : '↓' }} {{ x.name }}</span>
              <span :class="x.error ? 'text-red-300' : x.done ? 'text-green-300' : 'text-white/60'">
                {{
                  x.error ||
                  (x.done ? t('files.done') : x.total ? pct(x) + '%' : (x.bytes / 1024).toFixed(0) + ' KiB')
                }}
              </span>
            </div>
            <div v-if="!x.done && x.total" class="mt-1 h-1 bg-white/10">
              <div class="h-full bg-white/70" :style="{ width: pct(x) + '%' }" />
            </div>
          </li>
        </ul>
      </div>
      <div class="grid gap-2">
        <button
          class="flex items-center gap-2 border border-white/30 px-3 py-2 text-sm hover:bg-white/10"
          @click="ctrlAltDel"
        >
          <Keyboard class="h-4 w-4" /> {{ t('client.ctrlAltDel') }}
        </button>
        <button
          class="flex items-center gap-2 border border-white/30 px-3 py-2 text-sm hover:bg-white/10"
          @click="toggleFullscreen"
        >
          <component :is="fullscreen ? Minimize : Maximize" class="h-4 w-4" />
          {{ fullscreen ? t('client.exitFullscreen') : t('client.fullscreen') }}
        </button>
        <button
          class="flex items-center gap-2 border border-red-400/60 px-3 py-2 text-sm text-red-300 hover:bg-red-500/10"
          data-testid="disconnect"
          @click="disconnect"
        >
          <LogOut class="h-4 w-4" /> {{ t('client.disconnect') }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.client-view :deep(*) {
  text-shadow: none;
}
</style>
