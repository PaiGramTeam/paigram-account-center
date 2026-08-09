import type { createRequest } from '../request'
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

interface BackendRole {
  id: number
  name: string
  display_name: string
  description: string
  is_system?: boolean
  permissions?: Array<{
    id: number
    name: string
    resource: string
    action: string
    description: string
    created_at?: string
    updated_at?: string
  }>
  created_at: string
  updated_at: string
}

function mapRole(role: BackendRole): RoleListItem {
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

function mapRoleDetail(role: BackendRole) {
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
      created_at: permission.created_at,
      updated_at: permission.updated_at,
    })),
    updated_at: role.updated_at,
  }
}

export function createRoleApi(request: ReturnType<typeof createRequest>) {
  return {
    async getList(params?: RoleListParams): Promise<RoleListResponse> {
      const response = await request.get<{ data: { data: BackendRole[]; pagination: RoleListResponse['pagination'] } }>(
        '/roles',
        { params }
      )
      return {
        data: response.data.data.data.map(mapRole),
        pagination: response.data.data.pagination,
      }
    },

    async getDetail(id: number | string): Promise<RoleDetailResponse> {
      const response = await request.get<BackendRole>(`/roles/${id}`)
      return {
        data: mapRoleDetail(response.data),
      }
    },

    async create(data: CreateRoleRequest): Promise<CreateRoleResponse> {
      return request.post('/roles', data)
    },

    async update(id: number | string, data: UpdateRoleRequest): Promise<UpdateRoleResponse> {
      return request.put(`/roles/${id}`, data)
    },

    async delete(id: number | string): Promise<DeleteRoleResponse> {
      return request.delete(`/roles/${id}`)
    },
  }
}
