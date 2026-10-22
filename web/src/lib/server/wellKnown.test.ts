import { describe, expect, it, vi } from 'vitest';

vi.mock('$app/env/private', () => ({ API_INTERNAL_URL: 'http://api', SSR_API_TIMEOUT_MS: undefined }));
vi.mock('#lib/server/api.js', () => ({ ssrTimeoutMs: () => 1000 }));

const { proxyDiscoveryDocument } = await import('./wellKnown');

function upstream(status: number, body = '{}'): typeof globalThis.fetch {
  return vi.fn(async () => new Response(body, { status })) as unknown as typeof globalThis.fetch;
}

describe('proxyDiscoveryDocument', () => {
  it('serves the document under the content type the spec names', async () => {
    const fetch = upstream(200, '{"keys":[]}');

    const res = await proxyDiscoveryDocument(fetch, '/api/v1/ojcp/keys', 'application/jwk-set+json');

    expect(fetch).toHaveBeenCalledWith('http://api/api/v1/ojcp/keys', expect.anything());
    expect(res.status).toBe(200);
    expect(res.headers.get('content-type')).toBe('application/jwk-set+json');
    expect(res.headers.get('access-control-allow-origin')).toBe('*');
    expect(await res.text()).toBe('{"keys":[]}');
  });

  it('passes a 404 through, so "none published" is not read as "provider down"', async () => {
    const res = await proxyDiscoveryDocument(upstream(404), '/api/v1/ojcp/keys', 'application/jwk-set+json');

    expect(res.status).toBe(404);
  });

  it('refuses with 503 when the backend fails', async () => {
    const failing = vi.fn(async () => {
      throw new Error('timeout');
    }) as unknown as typeof globalThis.fetch;

    expect((await proxyDiscoveryDocument(upstream(500), '/x', 'application/json')).status).toBe(503);
    expect((await proxyDiscoveryDocument(failing, '/x', 'application/json')).status).toBe(503);
  });
});
