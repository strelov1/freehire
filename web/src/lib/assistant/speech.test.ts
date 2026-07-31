import { describe, it, expect, vi, afterEach } from 'vitest';
import { transcribe, TranscriptionUnavailable } from './speech';

/** Stub `fetch`, returning what it was called with so the request can be asserted. */
function stubFetch(status: number, body: unknown) {
  const spy = vi.fn(async () =>
    new Response(typeof body === 'string' ? body : JSON.stringify(body), {
      status,
      headers: { 'content-type': 'application/json' },
    }),
  );
  vi.stubGlobal('fetch', spy);
  return spy;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('transcribe', () => {
  it('posts the recording as multipart and returns the text', async () => {
    const spy = stubFetch(200, { data: { text: 'compare the first two' } });

    const text = await transcribe(new Blob(['opus']), 'dictation.webm');
    expect(text).toBe('compare the first two');

    const [url, init] = spy.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/v1/speech/transcriptions');
    expect(init.method).toBe('POST');
    expect(init.body).toBeInstanceOf(FormData);
    // No content-type is set by hand: the browser generates the multipart boundary,
    // and a hand-written header would send a body the server cannot parse.
    expect(init.headers).toBeUndefined();
    const form = init.body as FormData;
    expect((form.get('file') as File).name).toBe('dictation.webm');
  });

  it('reports an unconfigured gateway as its own type, not as a failure', async () => {
    stubFetch(501, { error: 'transcription is not configured' });
    await expect(transcribe(new Blob(['opus']), 'a.webm')).rejects.toBeInstanceOf(
      TranscriptionUnavailable,
    );
  });

  it('explains a throttled caller rather than showing them a status code', async () => {
    stubFetch(429, { error: 'too many requests' });
    await expect(transcribe(new Blob(['opus']), 'a.webm')).rejects.toThrow(/try again/i);
  });

  it('fails on any other refusal', async () => {
    stubFetch(502, { error: 'transcription failed' });
    await expect(transcribe(new Blob(['opus']), 'a.webm')).rejects.toThrow(/502/);
  });

  it('treats silence as an empty transcription rather than as an error', async () => {
    stubFetch(200, { data: { text: '' } });
    await expect(transcribe(new Blob(['opus']), 'a.webm')).resolves.toBe('');
  });

  it('survives a response with no text at all', async () => {
    stubFetch(200, { data: {} });
    await expect(transcribe(new Blob(['opus']), 'a.webm')).resolves.toBe('');
  });
});
