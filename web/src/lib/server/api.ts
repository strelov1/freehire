import { env } from '$env/dynamic/private';
import { createApi } from '$lib/api';

/** Build an API client for use inside a SvelteKit server `load` or endpoint.
 *  Pass the event's `fetch`, and the incoming request's `cookie` header for any
 *  authenticated call (e.g. `/me`, `/my/*`).
 *
 *  In production set `API_INTERNAL_URL` to the Go service origin (e.g.
 *  `http://app:8080`) so server-to-server calls reach the backend directly — a
 *  relative `/api` would hit this Node server, which has no such route. In dev
 *  it defaults to relative URLs, which the Vite proxy forwards to the backend.
 *
 *  With an absolute base URL, `event.fetch` does not forward the request's
 *  cookies, so the session cookie is forwarded explicitly when provided. Public
 *  reads can omit it. */
export function serverApi(fetchImpl: typeof fetch, cookie?: string | null) {
  return createApi(fetchImpl, env.API_INTERNAL_URL ?? '', cookie ? { cookie } : {}, timeoutMs());
}

/** How long a server-side API call may take before it is aborted. Every SSR call is
 *  a page waiting to render, so this is deliberately far below nginx's 60s
 *  `proxy_read_timeout`: a `load` that outlives that deadline has already lost its
 *  reader, and left unbounded it holds its socket open indefinitely. Enough such
 *  sockets fill the accept queue and the SSR process stops answering at all — the
 *  outage of 2026-08-01, where nginx returned 504 while the node held 499 sockets in
 *  CLOSE-WAIT. `SSR_API_TIMEOUT_MS` overrides it if a slow endpoint ever needs room. */
const DEFAULT_TIMEOUT_MS = 10_000;

/** The same deadline, for a server-side caller that does not go through `createApi` — the
 *  OJCP manifest proxy, which forwards one raw JSON document rather than calling a typed
 *  endpoint. Exported so there is one figure rather than two that drift. */
export function ssrTimeoutMs(): number {
  return timeoutMs();
}

function timeoutMs(): number {
  const configured = Number(env.SSR_API_TIMEOUT_MS);
  return Number.isFinite(configured) && configured > 0 ? configured : DEFAULT_TIMEOUT_MS;
}
