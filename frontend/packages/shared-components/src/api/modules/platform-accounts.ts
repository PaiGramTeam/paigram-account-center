import type { createRequest } from '../request'
import type { components } from '../generated/schema'

export type PlatformBinding = components['schemas']['BindingView']
export type AdminPlatformBinding = components['schemas']['AdminBindingView']
export type PlatformProfile = components['schemas']['ProfileView']
export type ConsumerGrant = components['schemas']['ConsumerGrantView']
export type PlatformRuntimeSummary = components['schemas']['RuntimeSummary']
export type PlatformDefinition = components['schemas']['PlatformListView']
export type PlatformSchema = components['schemas']['PlatformSchemaView']
export type Pagination = components['schemas']['PaginationMeta']
export type DashboardSummary = components['schemas']['DashboardSummaryView']
export type BotIdentity = components['schemas']['BotIdentityDTO']
export type EntryIdentityChallenge = components['schemas']['EntryIdentityChallengeView']
export type EntryIdentityUnlinkResult = components['schemas']['UnlinkResult']
export type OperationRecovery = components['schemas']['OperationRecoveryView']

export interface PlatformBindingList<T> {
  items: T[] | null
  pagination: Pagination
}

export interface CreatePlatformBindingInput {
  platform: string
  display_name: string
  credential_payload: Record<string, unknown>
}

export interface GrantChangeInput {
  enabled: boolean
  actions?: string[]
}

export function createPlatformAccountsApi(request: ReturnType<typeof createRequest>) {
  return {
    async getDashboardSummary(): Promise<{ data: DashboardSummary }> {
      return request.get('/me/dashboard-summary')
    },

    async listBotIdentities(): Promise<{ data: BotIdentity[] | null }> {
      return request.get('/me/bot-identities')
    },

    async removeBotIdentity(
      botId: string,
      operationId: string
    ): Promise<{ data: EntryIdentityUnlinkResult } | undefined> {
      return request.delete(`/me/bot-identities/${encodeURIComponent(botId)}`, {
        params: { operation_id: operationId },
      })
    },

    async getBotIdentityUnlinkStatus(botId: string, operationId: string): Promise<{ data: EntryIdentityUnlinkResult }> {
      return request.get(`/me/bot-identities/${encodeURIComponent(botId)}/unlink-status`, {
        params: { operation_id: operationId },
        skipErrorToast: true,
      })
    },

    async previewEntryIdentityLink(challenge: string): Promise<{ data: EntryIdentityChallenge }> {
      return request.post('/me/entry-identity-links/preview', { challenge }, { skipErrorToast: true })
    },

    async approveEntryIdentityLink(challenge: string): Promise<{ data: BotIdentity }> {
      return request.post('/me/entry-identity-links/approve', { challenge }, { skipErrorToast: true })
    },

    async cancelEntryIdentityLink(challenge: string): Promise<void> {
      await request.post('/me/entry-identity-links/cancel', { challenge }, { skipErrorToast: true })
    },

    async listPlatforms(): Promise<{ data: PlatformDefinition[] | null }> {
      return request.get('/me/platforms')
    },

    async getPlatformSchema(platform: string): Promise<{ data: PlatformSchema }> {
      return request.get(`/me/platforms/${encodeURIComponent(platform)}/schema`)
    },

    async list(params?: {
      page?: number
      page_size?: number
    }): Promise<{ data: PlatformBindingList<PlatformBinding> }> {
      return request.get('/me/platform-accounts', { params })
    },

    async create(data: CreatePlatformBindingInput): Promise<{ data: PlatformBinding }> {
      return request.post('/me/platform-accounts', data)
    },

    async get(bindingId: number): Promise<{ data: PlatformBinding }> {
      return request.get(`/me/platform-accounts/${bindingId}`)
    },

    async updateCredential(
      bindingId: number,
      credential: Record<string, unknown>
    ): Promise<{ data: PlatformRuntimeSummary }> {
      return request.put(`/me/platform-accounts/${bindingId}/credential`, credential)
    },

    async refresh(bindingId: number): Promise<{ data: PlatformRuntimeSummary }> {
      return request.post(`/me/platform-accounts/${bindingId}/refresh`)
    },

    async remove(bindingId: number): Promise<{ data: PlatformBinding }> {
      return request.delete(`/me/platform-accounts/${bindingId}`)
    },

    async listProfiles(bindingId: number): Promise<{ data: PlatformBindingList<PlatformProfile> }> {
      return request.get(`/me/platform-accounts/${bindingId}/profiles`)
    },

    async setPrimaryProfile(bindingId: number, profileId: number): Promise<{ data: PlatformBinding }> {
      return request.patch(`/me/platform-accounts/${bindingId}/primary-profile`, { profile_id: profileId })
    },

    async listGrants(bindingId: number): Promise<{ data: PlatformBindingList<ConsumerGrant> }> {
      return request.get(`/me/platform-accounts/${bindingId}/consumer-grants`)
    },

    async changeGrant(bindingId: number, consumer: string, data: GrantChangeInput): Promise<{ data: ConsumerGrant }> {
      return request.put(`/me/platform-accounts/${bindingId}/consumer-grants/${encodeURIComponent(consumer)}`, data)
    },

    async getRuntimeSummary(bindingId: number): Promise<{ data: PlatformRuntimeSummary }> {
      return request.get(`/me/platform-accounts/${bindingId}/runtime-summary`)
    },
  }
}

export function createAdminPlatformAccountsApi(request: ReturnType<typeof createRequest>) {
  return {
    async list(params?: {
      page?: number
      page_size?: number
    }): Promise<{ data: PlatformBindingList<AdminPlatformBinding> }> {
      return request.get('/admin/platform-accounts', { params })
    },

    async get(bindingId: number): Promise<{ data: AdminPlatformBinding }> {
      return request.get(`/admin/platform-accounts/${bindingId}`)
    },

    async listProfiles(bindingId: number): Promise<{ data: PlatformBindingList<PlatformProfile> }> {
      return request.get(`/admin/platform-accounts/${bindingId}/profiles`)
    },

    async listGrants(bindingId: number): Promise<{ data: PlatformBindingList<ConsumerGrant> }> {
      return request.get(`/admin/platform-accounts/${bindingId}/consumer-grants`)
    },

    async listOperations(
      bindingId: number,
      params?: { page?: number; page_size?: number }
    ): Promise<{ data: PlatformBindingList<OperationRecovery> }> {
      return request.get(`/admin/platform-accounts/${bindingId}/operations`, { params })
    },

    async requeueOperation(bindingId: number, operationId: string): Promise<{ data: OperationRecovery }> {
      return request.post(`/admin/platform-accounts/${bindingId}/operations/${encodeURIComponent(operationId)}/requeue`)
    },

    async changeGrant(bindingId: number, consumer: string, data: GrantChangeInput): Promise<{ data: ConsumerGrant }> {
      return request.put(`/admin/platform-accounts/${bindingId}/consumer-grants/${encodeURIComponent(consumer)}`, data)
    },

    async updateCredential(
      bindingId: number,
      credential: Record<string, unknown>
    ): Promise<{ data: PlatformRuntimeSummary }> {
      return request.put(`/admin/platform-accounts/${bindingId}/credential`, credential)
    },

    async refresh(bindingId: number): Promise<{ data: PlatformRuntimeSummary }> {
      return request.post(`/admin/platform-accounts/${bindingId}/refresh`)
    },

    async getRuntimeSummary(bindingId: number): Promise<{ data: PlatformRuntimeSummary }> {
      return request.get(`/admin/platform-accounts/${bindingId}/runtime-summary`)
    },

    async remove(bindingId: number): Promise<{ data: AdminPlatformBinding }> {
      return request.delete(`/admin/platform-accounts/${bindingId}`)
    },
  }
}
