import { describe, expect, test } from 'bun:test'

interface OpenApiDocument {
  openapi: string
  paths: Record<string, unknown>
}

describe('account-center OpenAPI contract', () => {
  test('contains the frontend authentication, self-service, and administration routes', async () => {
    const contract = (await Bun.file('../contracts/openapi.json').json()) as OpenApiDocument

    expect(contract.openapi).toStartWith('3.')
    expect(contract.paths).toHaveProperty('/api/v1/auth/login')
    expect(contract.paths).toHaveProperty('/api/v1/me')
    expect(contract.paths).toHaveProperty('/api/v1/admin/users')
    expect(contract.paths).toHaveProperty('/api/v1/admin/roles')
    expect(contract.paths).toHaveProperty('/api/v1/me/entry-identity-links/approve')
    expect(contract.paths).toHaveProperty('/api/v1/me/bot-identities/{botId}/unlink-status')
    expect(contract.paths).toHaveProperty('/api/v1/admin/platform-accounts/{bindingId}/operations')
    expect(contract.paths).toHaveProperty(
      '/api/v1/admin/platform-accounts/{bindingId}/operations/{operationId}/requeue'
    )
  })
})
