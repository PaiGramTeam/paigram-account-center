export class AuthRefreshCoordinator<T> {
  private pending: Promise<T> | null = null

  run(refresh: () => Promise<T>, onFailure: () => void): Promise<T> {
    if (this.pending) {
      return this.pending
    }

    this.pending = refresh()
      .catch((error: unknown) => {
        onFailure()
        throw error
      })
      .finally(() => {
        this.pending = null
      })

    return this.pending
  }
}
