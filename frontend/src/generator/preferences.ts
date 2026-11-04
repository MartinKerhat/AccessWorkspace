// Stored settings are opaque JSON; only keys of the right type replace the
// current value. Shared by every place that generates with the user's
// remembered Generator settings (object form, Key Vault secret modal).
export function mergeStoredOptions<T extends object>(current: T, stored: Record<string, unknown> | undefined): T {
  if (!stored) {
    return current;
  }
  const next = { ...current } as Record<string, unknown>;
  for (const key of Object.keys(current)) {
    if (typeof stored[key] === typeof (current as Record<string, unknown>)[key]) {
      next[key] = stored[key];
    }
  }
  return next as T;
}
