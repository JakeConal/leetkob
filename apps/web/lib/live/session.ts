export const CSRF_STORAGE_KEY = "leetkob.live.csrf";

export function readCsrfToken(storage: { getItem(key: string): string | null }): string | null {
  const value = storage.getItem(CSRF_STORAGE_KEY);
  if (typeof value !== "string") return null;
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : null;
}

export function writeCsrfToken(
  storage: { setItem(key: string, value: string): void },
  token: string,
): void {
  storage.setItem(CSRF_STORAGE_KEY, token);
}

export function clearProtectedSession(
  storage: { removeItem(key: string): void },
  cache: { clear(): void },
): void {
  storage.removeItem(CSRF_STORAGE_KEY);
  cache.clear();
}
