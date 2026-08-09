function isUnhandledApiRequest(request: Request): boolean {
  return new URL(request.url).pathname.startsWith('/api/')
}

export async function enableMocking(): Promise<void> {
  if (import.meta.env.MODE !== 'mock' && import.meta.env.VITE_PAIGRAM_DATA_MODE !== 'mock') return

  const { worker } = await import('./browser')
  await worker.start({
    onUnhandledRequest(request, print) {
      if (isUnhandledApiRequest(request)) print.error()
    },
  })
}
