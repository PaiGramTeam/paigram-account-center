import { setupWorker } from 'msw/browser'
import { readMockOptions } from './config'
import { createMockHandlers } from './handlers'

export const mockOptions = readMockOptions()
export const worker = setupWorker(...createMockHandlers(mockOptions))
