import type { components } from '../generated/schema'
import type { createRequest } from '../request'

export type PlatformService = components['schemas']['PlatformServiceAdminView']
export type ServiceCredential = components['schemas']['CredentialView']
export type ServiceCredentialSecret = components['schemas']['CreateResult']

export interface PlatformServiceInput {
  platform_key: string
  display_name: string
  service_key: string
  service_audience: string
  discovery_type: string
  endpoint: string
  enabled: boolean
  supported_actions: string[]
  credential_schema: Record<string, unknown>
}

export interface ServiceCredentialInput {
  client_id: string
  bot_id: string
  display_name: string
  description: string
  audiences: string[]
  scopes: string[]
}

export function createAdminServicesApi(request: ReturnType<typeof createRequest>) {
  return {
    async listPlatformServices(): Promise<{ data: PlatformService[] | null }> {
      return request.get('/admin/system/platform-services')
    },

    async createPlatformService(data: PlatformServiceInput): Promise<{ data: PlatformService }> {
      return request.post('/admin/system/platform-services', data)
    },

    async updatePlatformService(id: number, data: Partial<PlatformServiceInput>): Promise<{ data: PlatformService }> {
      return request.patch(`/admin/system/platform-services/${id}`, data)
    },

    async checkPlatformService(id: number): Promise<{ data: PlatformService }> {
      return request.post(`/admin/system/platform-services/${id}/check`)
    },

    async deletePlatformService(id: number): Promise<void> {
      await request.delete(`/admin/system/platform-services/${id}`)
    },

    async listServiceCredentials(): Promise<{ data: ServiceCredential[] | null }> {
      return request.get('/admin/service-credentials')
    },

    async createServiceCredential(data: ServiceCredentialInput): Promise<{ data: ServiceCredentialSecret }> {
      return request.post('/admin/service-credentials', data)
    },

    async rotateServiceCredential(clientId: string): Promise<{ data: ServiceCredentialSecret }> {
      return request.post(`/admin/service-credentials/${encodeURIComponent(clientId)}/secret`)
    },
  }
}
