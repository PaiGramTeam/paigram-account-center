export class AuthRefreshCoordinator<T> {
  private pending: Promise<T> | null = null

  run(refresh: () => Promise<T>, onFailure: (error: unknown) => void | Promise<void>): Promise<T> {
    if (this.pending) {
      return this.pending
    }

    this.pending = refresh()
      .catch(async (error: unknown) => {
        await onFailure(error)
        throw error
      })
      .finally(() => {
        this.pending = null
      })

    return this.pending
  }
}
