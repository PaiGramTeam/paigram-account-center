import type { createRequest } from '../request'
import type { OpenApiRole } from '../openapi'
import type {
  RoleListParams,
  RoleListItem,
  RoleListResponse,
  CreateRoleRequest,
  CreateRoleResponse,
  UpdateRoleRequest,
  UpdateRoleResponse,
  DeleteRoleResponse,
  RoleDetailResponse,
} from '../types'

function mapRole(role: OpenApiRole): RoleListItem {
  return {
    id: role.id,
    name: role.name,
    display_name: role.display_name,
    description: role.description,
    permission_count: role.permissions?.length ?? 0,
    user_count: undefined,
    is_system: role.is_system,
    created_at: role.created_at,
  }
}

function mapRoleDetail(role: OpenApiRole) {
  return {
    ...mapRole(role),
    permissions: (role.permissions || []).map((permission) => ({
      id: permission.id,
      name: permission.name,
      display_name: permission.name,
      description: permission.description,
      category: permission.resource,
      resource: permission.resource,
      action: permission.action,
      created_at: undefined,
      updated_at: undefined,
    })),
    updated_at: role.updated_at,
  }
}

export function createRoleApi(request: ReturnType<typeof createRequest>) {
  return {
    async getList(params?: RoleListParams): Promise<RoleListResponse> {
      const response = await request.get<{
        items: OpenApiRole[] | null
        pagination: RoleListResponse['pagination']
      }>('/admin/roles', { params })
      return {
        data: (response.data.items ?? []).map(mapRole),
        pagination: response.data.pagination,
      }
    },

    async getDetail(id: number | string): Promise<RoleDetailResponse> {
      const response = await request.get<OpenApiRole>(`/admin/roles/${id}`)
      return {
        data: mapRoleDetail(response.data),
      }
    },

    async create(data: CreateRoleRequest): Promise<CreateRoleResponse> {
      return request.post('/admin/roles', data)
    },

    async update(id: number | string, data: UpdateRoleRequest): Promise<UpdateRoleResponse> {
      return request.put(`/admin/roles/${id}`, data)
    },

    async delete(id: number | string): Promise<DeleteRoleResponse> {
      return request.delete(`/admin/roles/${id}`)
    },
  }
}
