export const langStorageKey = "roodox.workbench.lang";
export const accessOnboardingStorageKey = "roodox.workbench.access-onboarding.v1";
export const joinRequestStorageKey = "roodox.workbench.join-request.v1";
export const connectionCodeStorageKey = "roodox.workbench.connection-code.v1";

export const workbenchStorageKeys = [
  langStorageKey,
  accessOnboardingStorageKey,
  joinRequestStorageKey,
  connectionCodeStorageKey
];

function getLocalStorage(): Storage | null {
  try {
    return typeof window === "undefined" ? null : window.localStorage;
  } catch {
    return null;
  }
}

function storageErrorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export function safeGetItem(key: string): string | null {
  const storage = getLocalStorage();
  if (!storage) return null;
  try {
    return storage.getItem(key);
  } catch {
    return null;
  }
}

export function safeSetItem(key: string, value: string): string | null {
  const storage = getLocalStorage();
  if (!storage) return "localStorage unavailable";
  try {
    storage.setItem(key, value);
    return null;
  } catch (error) {
    console.warn(`persist ${key} failed`, error);
    return storageErrorText(error);
  }
}

export function safeRemoveItem(key: string): string | null {
  const storage = getLocalStorage();
  if (!storage) return "localStorage unavailable";
  try {
    storage.removeItem(key);
    return null;
  } catch (error) {
    console.warn(`remove ${key} failed`, error);
    return storageErrorText(error);
  }
}
