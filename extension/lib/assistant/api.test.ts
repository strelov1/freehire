import { describe, it, expect, vi, afterEach } from 'vitest';
import { createSession, getSession, deleteSession, SessionNotFound } from './api';
import { must } from '../test-utils';

/** Stands in for the network, recording what the caller asked for. */
function stubFetch(response: { status: number; body?: unknown }) {
  const calls: { url: string; init: RequestInit }[] = [];
  vi.stubGlobal('fetch', async (url: string, init: RequestInit) => {
    calls.push({ url, init });
    return {
      ok: response.status >= 200 && response.status < 300,
      status: response.status,
      json: async () => response.body ?? {},
    } as Response;
  });
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

describe('the assistant API', () => {
  // The whole divergence from the web's client: extension code cannot see hire's
  // httpOnly cookie across origins, so the credential rides a header instead.
  // `credentials: 'omit'` is explicit, not incidental: host_permissions makes Chrome
  // attach a signed-in website tab's session cookie to this call by default, which
  // would make the request carry BOTH credentials and get misread server-side as
  // the website rather than the extension.
  it('presents the session JWT as a Bearer credential, not as a cookie', async () => {
    const calls = stubFetch({ status: 200, body: { data: { id: 's1', preset: 'browse', label: '' } } });

    await createSession('tok-123');

    expect(calls).toHaveLength(1);
    expect(must(calls[0]).init.headers).toMatchObject({ Authorization: 'Bearer tok-123' });
    expect(must(calls[0]).init.credentials).toBe('omit');
  });

  // The panel's agent is the one with eyes; a chat session would have no page tool.
  it('creates its conversation under the browsing preset', async () => {
    const calls = stubFetch({ status: 200, body: { data: { id: 's1', preset: 'browse', label: '' } } });

    await createSession('tok-123');

    expect(must(calls[0]).url).toContain('preset=browse');
    expect(must(calls[0]).init.method).toBe('POST');
  });

  it('reaches hire by absolute origin, since an extension page has none of its own', async () => {
    const calls = stubFetch({ status: 200, body: { data: { id: 's1', preset: 'browse', label: '' } } });

    await createSession('tok-123');

    expect(must(calls[0]).url).toMatch(/^https?:\/\/.+\/api\/v1\/assistant\/sessions/);
  });

  // A conversation deleted from the web is a dead id the panel holds. It has to be
  // distinguishable from a broken assistant, because the panel's answer differs:
  // start a fresh conversation silently, rather than report a failure.
  it('reports a missing conversation as its own error type', async () => {
    stubFetch({ status: 404 });

    await expect(getSession('gone', 'tok')).rejects.toBeInstanceOf(SessionNotFound);
  });

  it('reports any other failure as a plain error', async () => {
    stubFetch({ status: 500 });

    const err = await getSession('s1', 'tok').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(Error);
    expect(err).not.toBeInstanceOf(SessionNotFound);
  });

  it('deletes a conversation without expecting a body back', async () => {
    const calls = stubFetch({ status: 204 });

    await expect(deleteSession('s1', 'tok')).resolves.toBeUndefined();
    expect(must(calls[0]).init.method).toBe('DELETE');
  });
});
