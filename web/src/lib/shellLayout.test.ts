import { describe, expect, it } from 'vitest';
import { isAccountArea, isApiReferenceRoute, isFullBleedRoute, isWideHeaderRoute } from './shellLayout';

describe('isFullBleedRoute', () => {
  it('covers the agent, with and without a session id', () => {
    expect(isFullBleedRoute('/my/assistant')).toBe(true);
    expect(isFullBleedRoute('/my/assistant/0f0b1e3a-1c2d-4e5f-8a9b-0c1d2e3f4a5b')).toBe(true);
  });

  it('covers the tailor workspace', () => {
    expect(isFullBleedRoute('/tailor/senior-go-engineer-acme')).toBe(true);
  });

  it('does not cover the API reference — it keeps its footer', () => {
    expect(isFullBleedRoute('/docs/api')).toBe(false);
  });

  it('leaves the rest of the account shell centered', () => {
    expect(isFullBleedRoute('/my')).toBe(false);
    expect(isFullBleedRoute('/my/cvs')).toBe(false);
  });

  it('does not catch the tailor marketing page, which is a normal document', () => {
    expect(isFullBleedRoute('/features/tailor')).toBe(false);
  });

  it('does not match on a prefix that only looks like the agent', () => {
    expect(isFullBleedRoute('/my/assistants')).toBe(false);
  });
});

describe('isApiReferenceRoute', () => {
  it('covers both the external and internal API references', () => {
    expect(isApiReferenceRoute('/docs/api')).toBe(true);
    expect(isApiReferenceRoute('/docs/api/internal')).toBe(true);
  });

  it('does not catch a sub-path of the API reference', () => {
    expect(isApiReferenceRoute('/docs/api/jobs')).toBe(false);
  });

  it('leaves unrelated routes alone', () => {
    expect(isApiReferenceRoute('/my')).toBe(false);
  });
});

describe('isWideHeaderRoute', () => {
  it('covers everything isFullBleedRoute does', () => {
    expect(isWideHeaderRoute('/my/assistant')).toBe(true);
    expect(isWideHeaderRoute('/tailor/senior-go-engineer-acme')).toBe(true);
  });

  it('covers the API reference', () => {
    expect(isWideHeaderRoute('/docs/api')).toBe(true);
  });

  it('covers the internal API reference too', () => {
    expect(isWideHeaderRoute('/docs/api/internal')).toBe(true);
  });

  it('does not catch a sub-path of the API reference', () => {
    expect(isWideHeaderRoute('/docs/api/jobs')).toBe(false);
  });

  it('leaves the rest of the account shell centered', () => {
    expect(isWideHeaderRoute('/my')).toBe(false);
    expect(isWideHeaderRoute('/my/cvs')).toBe(false);
  });
});

describe('isAccountArea', () => {
  it('covers /my and everything under it', () => {
    expect(isAccountArea('/my')).toBe(true);
    expect(isAccountArea('/my/tracking')).toBe(true);
  });

  it('does not match on a prefix that only looks like it', () => {
    expect(isAccountArea('/mystery')).toBe(false);
    expect(isAccountArea('/jobs')).toBe(false);
  });
});
