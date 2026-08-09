import type { createRequest } from '../request'
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

/**
 */
export function createSecurityApi(request: ReturnType<typeof createRequest>) {
  return {
    /**
     */
    async changePassword(id: number | string, data: ChangePasswordRequest): Promise<ChangePasswordResponse> {
      return request.post(`/profiles/${id}/password/change`, data)
    },

    /**
     */
    async enable2FA(id: number | string): Promise<Enable2FAResponse> {
      return request.post(`/profiles/${id}/2fa/enable`)
    },

    /**
     */
    async confirm2FA(id: number | string, data: Confirm2FARequest): Promise<Confirm2FAResponse> {
      return request.post(`/profiles/${id}/2fa/confirm`, data)
    },

    /**
     */
    async disable2FA(id: number | string, data: Disable2FARequest): Promise<Disable2FAResponse> {
      return request.post(`/profiles/${id}/2fa/disable`, data)
    },

    /**
     */
    async getDevices(id: number | string): Promise<DevicesResponse> {
      return request.get(`/profiles/${id}/devices`)
    },

    /**
     */
    async removeDevice(id: number | string, deviceId: string): Promise<RemoveDeviceResponse> {
      return request.delete(`/profiles/${id}/devices/${deviceId}`)
    },
  }
}
