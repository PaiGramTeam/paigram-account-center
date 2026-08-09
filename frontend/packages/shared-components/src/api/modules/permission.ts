import type { createRequest } from '../request'
import type { OpenApiPermissionInfo, OpenApiRole } from '../openapi'
import type { PermissionListParams, PermissionListResponse, PermissionDetailResponse } from '../types'

function toTitleCase(value: string): string {
  return value
    .split(/[_:-]/g)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function mapPermission(permission: OpenApiPermissionInfo) {
  return {
    id: permission.id,
    name: permission.name,
    display_name: `${toTitleCase(permission.resource)} ${toTitleCase(permission.action)}`,
    description: permission.description,
    category: permission.resource,
    resource: permission.resource,
    action: permission.action,
  }
}

export function createPermissionApi(request: ReturnType<typeof createRequest>) {
  async function listAssignedPermissions() {
    const response = await request.get<{ items: OpenApiRole[] | null }>('/admin/roles', {
      params: { page: 1, page_size: 100 },
    })
    const permissions = new Map<number, OpenApiPermissionInfo>()
    for (const role of response.data.items ?? []) {
      for (const permission of role.permissions ?? []) permissions.set(permission.id, permission)
    }
    return [...permissions.values()]
  }

  return {
    async getList(params?: PermissionListParams): Promise<PermissionListResponse> {
      const assignedPermissions = await listAssignedPermissions()
      const filteredPermissions = params?.category
        ? assignedPermissions.filter((permission) => permission.resource === params.category)
        : assignedPermissions
      const page = params?.page ?? 1
      const pageSize = params?.page_size ?? 20
      const offset = (page - 1) * pageSize

      return {
        data: filteredPermissions.slice(offset, offset + pageSize).map(mapPermission),
        pagination: {
          page,
          page_size: pageSize,
          total: filteredPermissions.length,
          total_pages: Math.max(1, Math.ceil(filteredPermissions.length / pageSize)),
        },
      }
    },

    async getDetail(id: number | string): Promise<PermissionDetailResponse> {
      const permission = (await listAssignedPermissions()).find((item) => String(item.id) === String(id))
      if (!permission) throw new Error('Permission not found in the assigned role catalog.')
      return {
        data: {
          ...mapPermission(permission),
          created_at: '',
          updated_at: '',
        },
      }
    },
  }
}
