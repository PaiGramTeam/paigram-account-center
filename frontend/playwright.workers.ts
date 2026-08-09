export function resolveWorkers(variable: string, fallback: number | string): number | string {
  const configured = process.env[variable]
  if (!configured) return fallback

  const percentage = configured.match(/^(\d+)%$/)
  if (percentage) {
    const value = Number(percentage[1])
    if (value >= 1 && value <= 100) return configured
  }

  const workers = Number(configured)
  if (Number.isInteger(workers) && workers > 0) return workers

  throw new Error(`${variable} must be a positive integer or CPU percentage`)
}
