import type { createRequest } from '../request'
import type { components } from '../generated/schema'

export type AuditEvent = components['schemas']['AuditEventView']
export type SystemSettings = components['schemas']['SettingsView']

export function createAdminSystemApi(request: ReturnType<typeof createRequest>) {
  const settingsPath = (domain: string): string =>
    domain === 'auth-controls' ? '/admin/system/auth-controls' : `/admin/system/settings/${domain}`

  return {
    async listAuditEvents(params?: {
      page?: number
      page_size?: number
      category?: string
      result?: string
    }): Promise<{ data: components['schemas']['PaginatedDataAuditEventView'] }> {
      return request.get('/admin/audit-logs', { params })
    },

    async getSettings(domain: string): Promise<{ data: SystemSettings }> {
      return request.get(settingsPath(domain))
    },

    async patchSettings(domain: string, data: Record<string, unknown>): Promise<{ data: SystemSettings }> {
      return request.patch(settingsPath(domain), data)
    },
  }
}
