import { computed } from 'vue'
import { useUserStore } from '../stores/user'

export function usePermission() {
  const userStore = useUserStore()

  const hasPermission = (permission: string | string[]): boolean => {
    if (!permission) return true

    const permissions = Array.isArray(permission) ? permission : [permission]
    return permissions.some((p) => userStore.hasPermission(p))
  }

  const hasRole = (role: string | string[]): boolean => {
    if (!role) return true

    const roles = Array.isArray(role) ? role : [role]
    return roles.some((r) => userStore.hasRole(r))
  }

  const hasAny = (options: { permissions?: string[]; roles?: string[] }): boolean => {
    const { permissions = [], roles = [] } = options

    if (permissions.length === 0 && roles.length === 0) return true

    return permissions.some((p) => hasPermission(p)) || roles.some((r) => hasRole(r))
  }

  const hasAll = (options: { permissions?: string[]; roles?: string[] }): boolean => {
    const { permissions = [], roles = [] } = options

    if (permissions.length === 0 && roles.length === 0) return true

    const hasAllPermissions = permissions.length === 0 || permissions.every((p) => hasPermission(p))
    const hasAllRoles = roles.length === 0 || roles.every((r) => hasRole(r))

    return hasAllPermissions && hasAllRoles
  }

  return {
    hasPermission,
    hasRole,
    hasAny,
    hasAll,
    permissions: computed(() => userStore.permissions),
    roles: computed(() => userStore.roles),
  }
}
