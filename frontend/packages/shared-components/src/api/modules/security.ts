import type { createRequest } from '../request'
import type { components } from '../generated/schema'
import type {
  ChangePasswordRequest,
  ChangePasswordResponse,
  Enable2FAResponse,
  Confirm2FARequest,
  Confirm2FAResponse,
  Disable2FARequest,
  Disable2FAResponse,
  DevicesResponse,
  RemoveDeviceResponse,
} from '../types'

export type SecurityOverview = components['schemas']['SecurityOverview']

/**
 */
export function createSecurityApi(request: ReturnType<typeof createRequest>) {
  return {
    /**
     */
    async changePassword(_id: number | string, data: ChangePasswordRequest): Promise<ChangePasswordResponse> {
      return request.put('/me/security/password', data)
    },

    /**
     */
    async enable2FA(_id: number | string, password: string): Promise<Enable2FAResponse> {
      const response = await request.post<components['schemas']['TwoFactorSetupView']>('/me/security/2fa/setup', {
        password,
      })
      return {
        data: {
          qr_code: response.data.qr_code,
          secret: response.data.secret,
          backup_codes: response.data.backup_codes ?? [],
        },
      }
    },

    /**
     */
    async confirm2FA(_id: number | string, data: Confirm2FARequest): Promise<Confirm2FAResponse> {
      return request.post('/me/security/2fa/confirm', { code: data.code })
    },

    /**
     */
    async disable2FA(_id: number | string, data: Disable2FARequest): Promise<Disable2FAResponse> {
      return request.delete('/me/security/2fa', { data })
    },

    /**
     */
    async getDevices(_id: number | string): Promise<DevicesResponse> {
      const response = await request.get<components['schemas']['PaginatedDataSessionView']>('/me/sessions')
      return {
        data: {
          data: (response.data.items ?? []).map((session) => ({
            device_id: String(session.id),
            device_name: session.device_name ?? 'Unknown device',
            device_type: session.device_type,
            ip: session.ip ?? '',
            location: session.location,
            last_active_at: session.last_active_at ?? session.created_at,
            is_current: session.is_current,
          })),
        },
      }
    },

    /**
     */
    async removeDevice(_id: number | string, deviceId: string): Promise<RemoveDeviceResponse> {
      return request.delete(`/me/sessions/${deviceId}`)
    },

    async getOverview(): Promise<{ data: SecurityOverview }> {
      return request.get('/me/security/overview')
    },
  }
}
