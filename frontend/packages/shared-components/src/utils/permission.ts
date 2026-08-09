import type { App, Directive } from 'vue'
import { useUserStore } from '../stores/user'

// v-permission 指令
const permissionDirective: Directive = {
  mounted(el, binding) {
    const { value } = binding
    const userStore = useUserStore()

    if (value && value instanceof Array && value.length > 0) {
      const hasPermission = value.some((permission) => userStore.hasPermission(permission))

      if (!hasPermission) {
        el.parentNode && el.parentNode.removeChild(el)
      }
    } else if (value && typeof value === 'string') {
      if (!userStore.hasPermission(value)) {
        el.parentNode && el.parentNode.removeChild(el)
      }
    } else {
      throw new Error("需要权限! 例如: v-permission=\"['create','edit']\" 或 v-permission=\"'delete'\"")
    }
  },
}

// v-role 指令
const roleDirective: Directive = {
  mounted(el, binding) {
    const { value } = binding
    const userStore = useUserStore()

    if (value && value instanceof Array && value.length > 0) {
      const hasRole = value.some((role) => userStore.hasRole(role))

      if (!hasRole) {
        el.parentNode && el.parentNode.removeChild(el)
      }
    } else if (value && typeof value === 'string') {
      if (!userStore.hasRole(value)) {
        el.parentNode && el.parentNode.removeChild(el)
      }
    } else {
      throw new Error("需要角色! 例如: v-role=\"['admin','editor']\" 或 v-role=\"'user'\"")
    }
  },
}

export function setupPermissionDirective(app: App) {
  app.directive('permission', permissionDirective)
  app.directive('role', roleDirective)
}

export { permissionDirective, roleDirective }
