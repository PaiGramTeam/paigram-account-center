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
 * 安全相关 API
 */
export function createSecurityApi(request: ReturnType<typeof createRequest>) {
  return {
    /**
     * 修改密码
     * @param id 用户 ID
     * @param data 修改密码请求数据
     */
    async changePassword(id: number | string, data: ChangePasswordRequest): Promise<ChangePasswordResponse> {
      return request.post(`/profiles/${id}/password/change`, data)
    },

    /**
     * 启用双因素认证（2FA）
     * 生成 TOTP 密钥和 QR 码
     * @param id 用户 ID
     */
    async enable2FA(id: number | string): Promise<Enable2FAResponse> {
      return request.post(`/profiles/${id}/2fa/enable`)
    },

    /**
     * 确认启用双因素认证
     * 使用 TOTP 验证码确认
     * @param id 用户 ID
     * @param data 确认请求数据
     */
    async confirm2FA(id: number | string, data: Confirm2FARequest): Promise<Confirm2FAResponse> {
      return request.post(`/profiles/${id}/2fa/confirm`, data)
    },

    /**
     * 禁用双因素认证
     * 需要提供密码和当前的 TOTP 验证码
     * @param id 用户 ID
     * @param data 禁用请求数据
     */
    async disable2FA(id: number | string, data: Disable2FARequest): Promise<Disable2FAResponse> {
      return request.post(`/profiles/${id}/2fa/disable`, data)
    },

    /**
     * 获取登录设备列表
     * @param id 用户 ID
     */
    async getDevices(id: number | string): Promise<DevicesResponse> {
      return request.get(`/profiles/${id}/devices`)
    },

    /**
     * 移除登录设备
     * 会撤销该设备的所有会话
     * @param id 用户 ID
     * @param deviceId 设备 ID
     */
    async removeDevice(id: number | string, deviceId: string): Promise<RemoveDeviceResponse> {
      return request.delete(`/profiles/${id}/devices/${deviceId}`)
    },
  }
}
