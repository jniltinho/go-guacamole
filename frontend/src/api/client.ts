// Typed fetch wrapper for /api/v1. Adds CSRF headers, maps JSON errors to ApiError.

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public field?: string,
    public data?: Record<string, unknown>,
  ) {
    super(message)
  }
}

let csrfToken = ''
let onUnauthorized: (() => void) | null = null

export function setCsrfToken(t: string) {
  csrfToken = t
}

export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

export interface RequestOptions {
  query?: Record<string, string | number | boolean | undefined | null>
  body?: unknown
  signal?: AbortSignal
}

export async function request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
  let url = '/api/v1' + path
  if (opts.query) {
    const qs = new URLSearchParams()
    for (const [k, v] of Object.entries(opts.query)) {
      if (v !== undefined && v !== null && v !== '') qs.set(k, String(v))
    }
    const s = qs.toString()
    if (s) url += '?' + s
  }
  const headers: Record<string, string> = { Accept: 'application/json', 'X-Requested-With': 'goguacamole' }
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json'
  if (method !== 'GET' && csrfToken) headers['X-CSRF-Token'] = csrfToken
  let res: Response
  try {
    res = await fetch(url, {
      method,
      headers,
      credentials: 'same-origin',
      body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
      signal: opts.signal,
    })
  } catch (e) {
    if ((e as Error).name === 'AbortError') throw e
    throw new ApiError(0, 'NETWORK', 'network error')
  }
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    const err = new ApiError(
      res.status,
      data.code ?? 'INTERNAL',
      data.message ?? res.statusText,
      data.field,
      data.data,
    )
    if (res.status === 401 && path !== '/auth/login' && onUnauthorized) onUnauthorized()
    throw err
  }
  return data as T
}

export const api = {
  get: <T>(path: string, query?: RequestOptions['query'], signal?: AbortSignal) =>
    request<T>('GET', path, { query, signal }),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, { body: body ?? {} }),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, { body: body ?? {} }),
  del: <T>(path: string, query?: RequestOptions['query']) => request<T>('DELETE', path, { query }),
}
