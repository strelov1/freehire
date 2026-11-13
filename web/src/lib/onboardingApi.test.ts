import { describe, expect, it } from 'vitest';

import { ApiError, createApi } from './api';
import { ONBOARDING_DAILY_LIMIT_MESSAGE } from './api-methods/onboarding';

type Seen = { url?: string; init?: RequestInit };

function answering(seen: Seen, status: number, body: unknown): typeof fetch {
  return ((url: string, init?: RequestInit) => {
    seen.url = url;
    seen.init = init;
    return Promise.resolve(new Response(JSON.stringify(body), { status }));
  }) as unknown as typeof fetch;
}

const pdf = () => new File(['%PDF'], 'cv.pdf', { type: 'application/pdf' });

describe('onboarding CV parse', () => {
  it('posts the PDF as multipart field `file` and unwraps the envelope', async () => {
    const seen: Seen = {};
    const api = createApi(answering(seen, 200, { data: { first_name: 'Ana', skills: ['go'] } }));
    const parse = await api.onboardingParseCv(pdf());
    expect(seen.url).toBe('/api/v1/onboarding/cv');
    expect(seen.init?.method).toBe('POST');
    const body = seen.init?.body;
    expect(body).toBeInstanceOf(FormData);
    expect((body as FormData).get('file')).toBeInstanceOf(File);
    expect(parse.first_name).toBe('Ana');
  });

  it('maps a 429 to the daily-limit error', async () => {
    const api = createApi(answering({}, 429, { error: 'rate limited' }));
    const err = await api.onboardingParseCv(pdf()).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(429);
    expect((err as ApiError).message).toBe(ONBOARDING_DAILY_LIMIT_MESSAGE);
  });

  it('keeps any other refusal as the server worded it', async () => {
    const api = createApi(answering({}, 400, { error: 'not a readable PDF' }));
    const err = await api.onboardingParseCv(pdf()).catch((e: unknown) => e);
    expect((err as ApiError).status).toBe(400);
    expect((err as ApiError).message).toBe('not a readable PDF');
  });

  it('refuses an oversize PDF before sending it', async () => {
    const seen: Seen = {};
    const api = createApi(answering(seen, 200, { data: {} }));
    const big = new File([new Uint8Array(9 * 1024 * 1024)], 'cv.pdf', { type: 'application/pdf' });
    const err = await api.onboardingParseCv(big).catch((e: unknown) => e);
    expect((err as ApiError).status).toBe(413);
    expect(seen.url).toBeUndefined();
  });
});

describe('onboarding matches', () => {
  it('posts skills and the query as JSON and returns jobs with the total', async () => {
    const seen: Seen = {};
    const api = createApi(
      answering(seen, 200, {
        data: [{ job: { public_slug: 'a' }, match_percent: 80 }],
        meta: { total: 12, limit: 10, offset: 0 },
      }),
    );
    const res = await api.onboardingMatches(['go', 'kubernetes'], 'work_mode=remote&posted_within_days=7');
    expect(seen.url).toBe('/api/v1/onboarding/matches');
    expect(seen.init?.method).toBe('POST');
    expect(JSON.parse(seen.init?.body as string)).toEqual({
      skills: ['go', 'kubernetes'],
      query: 'work_mode=remote&posted_within_days=7',
    });
    expect(res.data[0]?.match_percent).toBe(80);
    expect(res.meta.total).toBe(12);
  });
});

describe('stored CV upload', () => {
  it('PUTs the PDF as multipart field `file`', async () => {
    const seen: Seen = {};
    const api = createApi(answering(seen, 200, { data: { enabled: true, present: true } }));
    await api.putResume(pdf());
    expect(seen.url).toBe('/api/v1/me/resume');
    expect(seen.init?.method).toBe('PUT');
    const body = seen.init?.body;
    expect(body).toBeInstanceOf(FormData);
    expect((body as FormData).get('file')).toBeInstanceOf(File);
  });

  // 501 is storage switched off: the CV was not stored, which the caller must hear.
  it('fails when storage is unconfigured', async () => {
    const api = createApi(answering({}, 501, { error: 'résumé storage is not available' }));
    const err = await api.putResume(pdf()).catch((e: unknown) => e);
    expect((err as ApiError).status).toBe(501);
  });
});

describe('survey', () => {
  it('PUTs the patch as JSON', async () => {
    const seen: Seen = {};
    const api = createApi(answering(seen, 200, { data: { biggest_challenges: ['english'] } }));
    await api.updateSurvey({ biggest_challenges: ['english'] });
    expect(seen.url).toBe('/api/v1/me/survey');
    expect(seen.init?.method).toBe('PUT');
    expect(JSON.parse(seen.init?.body as string)).toEqual({ biggest_challenges: ['english'] });
  });
});

describe('survey read', () => {
  it('GETs the stored answers', async () => {
    const seen: Seen = {};
    const api = createApi(answering(seen, 200, { data: { biggest_challenges: ['english'] } }));
    const got = await api.getSurvey();
    expect(seen.url).toBe('/api/v1/me/survey');
    expect(got.biggest_challenges).toEqual(['english']);
  });
});
