// A correlation ID belongs to one recent incident in one tab. Session storage carries
// it through a same-tab OAuth redirect without using an account or analytics identifier.
const key = 'freehire:signin-incident';
const ttl = 30 * 60_000;
const incidentID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

function read(storage: Storage, now: number): string | undefined {
  const value = storage.getItem(key);
  if (!value) return undefined;
  const [id, expires] = value.split(':');
  const expiry = Number(expires);
  if (id && incidentID.test(id) && expiry > now && expiry <= now + ttl) return id;
  storage.removeItem(key);
  return undefined;
}

export function markSigninIncident(): string | undefined {
  try {
    const now = Date.now();
    const storage = window.sessionStorage;
    const existing = read(storage, now);
    if (existing) return existing;

    const id = window.crypto.randomUUID();
    storage.setItem(key, id + ':' + (now + ttl));
    return id;
  } catch {
    // Telemetry still records the exception when tab storage is unavailable.
    return undefined;
  }
}

export function takeSigninIncident(): string | undefined {
  try {
    const storage = window.sessionStorage;
    const id = read(storage, Date.now());
    if (id) storage.removeItem(key);
    return id;
  } catch {
    // Some browsers block access to session storage.
    return undefined;
  }
}
