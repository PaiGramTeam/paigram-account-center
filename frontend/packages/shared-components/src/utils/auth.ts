import type { NormalizedApiError } from '../api/request'

interface AuthErrorShape {
  code?: string
  error?: string
  message?: string
}

export function resolveAuthErrorMessage(error: unknown, fallback = '操作失败，请稍后重试'): string {
  if (!error) {
    return fallback
  }

  const normalized = error as AuthErrorShape | NormalizedApiError
  const code = normalized.code || ''
  const message = normalized.message || normalized.error || ''

  if (code === 'CAPTCHA_REQUIRED') {
    return '请先完成安全验证'
  }

  if (code === 'CAPTCHA_FAILED') {
    return '安全验证失败，请重试'
  }

  if (message === 'invalid credentials') {
    return '邮箱或密码错误'
  }

  if (message === 'email not verified') {
    return '请先完成邮箱验证后再登录'
  }

  if (message === 'invalid 2FA code') {
    return '二级验证码或备用恢复码无效'
  }

  if (message === 'invalid oauth state') {
    return '登录授权状态已失效，请重新发起登录'
  }

  if (message === 'oauth state expired') {
    return '登录授权已过期，请重新发起登录'
  }

  if (message) {
    return message
  }

  return fallback
}
