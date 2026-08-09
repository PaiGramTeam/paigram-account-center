import type { createRequest } from '../request'
import type {
  PermissionListParams,
  PermissionListResponse,
  CreatePermissionRequest,
  CreatePermissionResponse,
  DeletePermissionResponse,
  PermissionDetailResponse,
} from '../types'

interface BackendPermission {
  id: number
  name: string
  resource: string
  action: string
  description: string
  created_at: string
  updated_at: string
}

function toTitleCase(value: string): string {
  return value
    .split(/[_:-]/g)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function mapPermission(permission: BackendPermission) {
  return {
    id: permission.id,
    name: permission.name,
    display_name: `${toTitleCase(permission.resource)} ${toTitleCase(permission.action)}`,
    description: permission.description,
    category: permission.resource,
    resource: permission.resource,
    action: permission.action,
    created_at: permission.created_at,
    updated_at: permission.updated_at,
  }
}

function normalizePermissionPayload(data: CreatePermissionRequest) {
  const resource = data.resource || data.name.split(':')[0] || ''
  const action = data.action || data.name.split(':')[1] || ''

  return {
    name: data.name,
    resource,
    action,
    description: data.description,
  }
}

export function createPermissionApi(request: ReturnType<typeof createRequest>) {
  return {
    // List permissions with pagination.
    async getList(params?: PermissionListParams): Promise<PermissionListResponse> {
      const response = await request.get<{
        data: { data: BackendPermission[]; pagination: PermissionListResponse['pagination'] }
      }>('/permissions', { params })

      const allPermissions = response.data.data.data.map(mapPermission)
      const filteredPermissions = params?.category
        ? allPermissions.filter((permission) => permission.category === params.category)
        : allPermissions

      return {
        data: filteredPermissions,
        pagination: {
          ...response.data.data.pagination,
          total: params?.category ? filteredPermissions.length : response.data.data.pagination.total,
        },
      }
    },

    // Get one permission.
    async getDetail(id: number | string): Promise<PermissionDetailResponse> {
      const response = await request.get<BackendPermission>(`/permissions/${id}`)
      return {
        data: mapPermission(response.data),
      }
    },

    // Create a permission.
    async create(data: CreatePermissionRequest): Promise<CreatePermissionResponse> {
      return request.post('/permissions', normalizePermissionPayload(data))
    },

    // Delete a permission.
    async delete(id: number | string): Promise<DeletePermissionResponse> {
      return request.delete(`/permissions/${id}`)
    },
  }
}
