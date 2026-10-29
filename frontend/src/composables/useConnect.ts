import { useRouter } from 'vue-router'

import type { Connection } from '../api/types'

/** Opens a connection in the client view (same tab). */
export function useConnect() {
  const router = useRouter()
  return {
    connect: (c: Pick<Connection, 'id'>) => router.push(`/sessions/${c.id}/client`),
  }
}
