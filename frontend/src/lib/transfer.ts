// File transfer over the Guacamole protocol (SFTP / RDP drive), like the Java
// client's ManagedFilesystem: directory listings, downloads and uploads.
import Guacamole from 'guacamole-common-js'

const OK = 0x0000

export interface FileEntry {
  name: string
  path: string
  dir: boolean
}

// Pass a reactive() Transfer: the callbacks mutate it and the UI follows.
export interface Transfer {
  name: string
  upload: boolean
  bytes: number
  total: number
  done: boolean
  error: string
}

export const basename = (p: string) => p.slice(p.lastIndexOf('/') + 1)
export const joinPath = (dir: string, name: string) => (dir.endsWith('/') ? dir : dir + '/') + name

/** Lists one directory (stream index) of a filesystem object, directories first. */
export function listDir(object: Guacamole.Object, path: string): Promise<FileEntry[]> {
  return new Promise((resolve, reject) => {
    object.requestInputStream(path, (stream, mimetype) => {
      if (mimetype !== Guacamole.Object.STREAM_INDEX_MIMETYPE) {
        stream.sendAck('Unexpected mimetype', 0x0301)
        reject(new Error('not a directory'))
        return
      }
      // JSONReader does not acknowledge blobs itself; guacd sends the next one per ack
      stream.sendAck('Ready', OK)
      const reader = new Guacamole.JSONReader(stream)
      reader.onprogress = () => stream.sendAck('Received', OK)
      reader.onend = () => {
        const index = reader.getJSON() as Record<string, string>
        const out = Object.entries(index).map(([p, m]) => ({
          name: basename(p),
          path: p,
          dir: m === Guacamole.Object.STREAM_INDEX_MIMETYPE,
        }))
        out.sort((a, b) => Number(b.dir) - Number(a.dir) || a.name.localeCompare(b.name))
        resolve(out)
      }
    })
  })
}

/** Receives a file stream into memory and hands it to the browser as a download. */
export function receive(stream: Guacamole.InputStream, mimetype: string, t: Transfer) {
  stream.sendAck('Ready', OK)
  // shortcut: the whole file is buffered in memory, fine up to a few hundred MB; stream to disk if that becomes a limit
  const reader = new Guacamole.BlobReader(stream, mimetype)
  reader.onprogress = (n) => {
    t.bytes += n
  }
  reader.onend = () => {
    t.done = true
    const url = URL.createObjectURL(reader.getBlob())
    const a = document.createElement('a')
    a.href = url
    a.download = t.name
    a.click()
    setTimeout(() => URL.revokeObjectURL(url), 10_000)
  }
}

/** Sends a local file over an output stream (already opened on the client or a filesystem object). */
export function send(stream: Guacamole.OutputStream, file: File, t: Transfer): Promise<void> {
  return new Promise((resolve) => {
    const fail = (msg: string) => {
      if (t.done) return
      t.error = msg
      t.done = true
      resolve()
    }
    // guacd answers the "file"/"put" request first (it may refuse, e.g. upload disabled)
    stream.onack = (first) => {
      if (first.isError()) return fail(first.message || 'error ' + first.code)
      start()
    }
    const start = () => {
      const writer = new Guacamole.BlobWriter(stream)
      writer.onack = (status) => {
        if (status.isError()) fail(status.message || 'error ' + status.code)
      }
      writer.onprogress = (_blob, offset) => {
        t.bytes = offset
      }
      writer.oncomplete = () => {
        writer.sendEnd()
        t.bytes = t.total
        t.done = true
        resolve()
      }
      writer.onerror = (_blob, _offset, err) => fail(err.message)
      writer.sendBlob(file)
    }
  })
}
