import { describe, expect, it } from 'vitest';

import { ApiError, createApi } from './api';
import { must } from './utils';

/** A fetch that never answers, and only settles if the caller aborts it. Stands in for
 *  the failure this timeout exists for: an API call that neither returns nor errors. */
function hangingFetch(): typeof fetch {
  return ((_url: string, init?: RequestInit) =>
    new Promise((_resolve, reject) => {
      init?.signal?.addEventListener('abort', () => reject(must(init.signal).reason));
    })) as unknown as typeof fetch;
}

/** A fetch that answers immediately with an empty JSON envelope. */
function okFetch(seen: { init?: RequestInit } = {}): typeof fetch {
  return ((_url: string, init?: RequestInit) => {
    seen.init = init;
    return Promise.resolve(new Response('{"data":null}', { status: 200 }));
  }) as unknown as typeof fetch;
}

describe('createApi request timeout', () => {
  // Without a bound timeout an unanswered call hangs forever, and on the server that
  // means the SvelteKit handler never finishes: nginx gives up at 60s and returns 504,
  // but Node keeps the socket in CLOSE-WAIT. Enough of those fill the accept queue and
  // the whole site stops responding — this is what took prod down on 2026-08-01.
  it('fails a hung call with 504 instead of hanging', async () => {
    const client = createApi(hangingFetch(), '', {}, 20);
    await expect(client.ingestStatus()).rejects.toBeInstanceOf(ApiError);
    await expect(client.ingestStatus()).rejects.toMatchObject({ status: 504 });
  });

  // The browser client is built without a timeout: it drives the LLM-backed calls
  // (match analysis, tailoring), which legitimately run for minutes.
  it('leaves the call unbounded when no timeout is configured', async () => {
    const seen: { init?: RequestInit } = {};
    await createApi(okFetch(seen), '', {}).ingestStatus();
    expect(seen.init?.signal).toBeUndefined();
  });

  it('arms the abort signal when a timeout is configured', async () => {
    const seen: { init?: RequestInit } = {};
    await createApi(okFetch(seen), '', {}, 5000).ingestStatus();
    expect(seen.init?.signal).toBeInstanceOf(AbortSignal);
  });

  // The timeout must not swallow the request body or method of a non-GET call — it is
  // added to the init, not substituted for it.
  it('keeps the rest of the request init intact', async () => {
    const seen: { init?: RequestInit } = {};
    await createApi(okFetch(seen), '', {}, 5000).login('a@b.co', 'pw');
    expect(seen.init?.method).toBe('POST');
    expect(seen.init?.body).toContain('a@b.co');
  });
});

describe('job path segment encoding', () => {
  // getJob/saveJob/recordJobView/voteJob/reportJob used to interpolate `slug` raw,
  // unlike their siblings a few hundred lines away (trackApplication,
  // getFollowUpDraft, recordFollowUp) which already wrap it in encodeURIComponent.
  // Slugs are dictionary-safe today, so this was latent — but a slug containing a
  // reserved character would silently mis-route the request.
  it('encodes a slug with reserved characters instead of splicing it into the path raw', async () => {
    const urls: string[] = [];
    const fetcher = ((url: string) => {
      urls.push(url);
      // A job-shaped body: getJob rejects anything else as "not a job" (see the
      // reserved-word test below), and this case is about the path, not the payload.
      return Promise.resolve(new Response('{"data":{"public_slug":"x"}}', { status: 200 }));
    }) as unknown as typeof fetch;
    const client = createApi(fetcher);
    const slug = 'a/b?c#d';
    const encoded = encodeURIComponent(slug);

    await client.getJob(slug);
    await client.saveJob(slug);
    await client.recordJobView(slug);
    await client.voteJob(slug, 'up');
    await client.reportJob(slug, { reason: 'other' });

    expect(urls).toEqual([
      `/api/v1/jobs/${encoded}`,
      `/api/v1/jobs/${encoded}/save`,
      `/api/v1/jobs/${encoded}/view`,
      `/api/v1/jobs/${encoded}/vote`,
      `/api/v1/jobs/${encoded}/reports`,
    ]);
  });

  // `/api/v1/jobs/{search,find,facets,sitemap}` are static routes declared ahead of
  // `/jobs/:slug`, so those four words answer 200 with their own body instead of the
  // 404 a missing job gives. /jobs/search in a browser therefore handed the detail
  // page a list of jobs where a posting belonged, and every one of the four rendered
  // a 500. The shape is what getJob checks, so a fifth static sibling added later is
  // covered without anyone remembering to add it here.
  it('treats a non-job body as a missing job', async () => {
    const listBody = '{"data":[{"public_slug":"a-job"}],"meta":{"total":1}}';
    const fetcher = (() =>
      Promise.resolve(new Response(listBody, { status: 200 }))) as unknown as typeof fetch;
    const client = createApi(fetcher);

    await expect(client.getJob('search')).rejects.toMatchObject({ status: 404 });
  });
});

describe('recent authentication adapters', () => {
  it('sends password reauthentication to the v2 cookie endpoint', async () => {
    let seenURL = '';
    let seenInit: RequestInit | undefined;
    const fetcher = ((url: string, init?: RequestInit) => {
      seenURL = url; seenInit = init;
      return Promise.resolve(new Response('{"data":{"recent_auth_expires_at":"2026-08-11T00:00:00Z"}}',{status:200}));
    }) as unknown as typeof fetch;
    const expires = await createApi(fetcher).reauthenticatePassword('correct horse');
    expect(seenURL).toBe('/api/v2/auth/reauth/password');
    expect(seenInit?.method).toBe('POST');
    expect(seenInit?.body).toContain('correct horse');
    expect(expires).toBe('2026-08-11T00:00:00Z');
  });

  // Go marshals an empty slice as `null`, so an account with no connected provider
  // answers `{"identities":null}` while the declared type promises an array. Every
  // caller filters the list to offer the "confirm with <provider>" buttons, and an
  // unguarded `.filter` on null turns "you have no providers" into "could not load
  // your providers" — the wrong story, on the screen that has to explain why an
  // action was refused.
  it('reads an empty identity list as an array, not null', async () => {
    const fetcher = (() =>
      Promise.resolve(new Response('{"data":{"has_password":true,"identities":null}}', { status: 200 }))
    ) as unknown as typeof fetch;
    const result = await createApi(fetcher).connectedIdentities();
    expect(result.identities).toEqual([]);
    expect(result.has_password).toBe(true);
  });
});
