export function isSafeInternalPath(path: string): boolean {
  return path.startsWith('/') && !path.startsWith('//') && !path.includes('\\')
}
