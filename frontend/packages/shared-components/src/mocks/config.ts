export const mockScenarios = ['default', 'error', 'slow'] as const

export type MockScenario = (typeof mockScenarios)[number]

export interface MockOptions {
  scenario: MockScenario
}

interface MockOptionSources {
  environment?: { scenario?: string }
  search?: string
}

export function resolveMockOptions(sources: MockOptionSources = {}): MockOptions {
  const query = new URLSearchParams(sources.search ?? '')
  const candidates = [query.get('mockScenario'), sources.environment?.scenario]

  return {
    scenario:
      candidates.find((value): value is MockScenario => mockScenarios.includes(value as MockScenario)) ?? 'default',
  }
}

export function readMockOptions(): MockOptions {
  return resolveMockOptions({
    environment: { scenario: import.meta.env.VITE_PAIGRAM_MOCK_SCENARIO },
    search: globalThis.location?.search,
  })
}
