import { describe, expect, it } from 'vitest';
import { EXIT_REFUSE, configurationOf, probeUrls, verdict } from './sentry-credential-check.mjs';

// The whole point of this check is that a release can tell apart states that used to look
// identical from the outside: uploading, deliberately not uploading, half-configured, and
// broken. Each case below is one of those, and the assertions are on the DECISION plus enough
// of the message to prove the release will say which one it is in.

describe('probeUrls', () => {
  // WHICH endpoint is asked is the single most load-bearing decision here, and it was wrong
  // once: the first draft probed the project's release list, which Sentry also grants to a
  // read-only `project:read` token, so an under-scoped credential would have passed the check
  // and failed the upload. Nothing caught it, because nothing asserted the URL.
  it('asks chunk-upload first — the call sentry-cli itself makes before uploading', () => {
    const [first] = probeUrls('https://sentry.io', 'diffray', 'freehire-web');
    expect(first).toBe('https://sentry.io/api/0/organizations/diffray/chunk-upload/');
  });

  // chunk-upload is organisation-scoped, so it cannot see a mistyped SENTRY_PROJECT — which
  // would upload into nothing, just as silently.
  it('also reads the project, which the organisation-scoped call cannot check', () => {
    const [, second] = probeUrls('https://sentry.io', 'diffray', 'freehire-web');
    expect(second).toContain('/api/0/projects/diffray/freehire-web/releases/');
  });

  // A hand-edited SENTRY_URL is as likely to carry a trailing slash as not.
  it('does not double the slash when the base URL carries one', () => {
    for (const url of probeUrls('https://de.sentry.io//', 'o', 'p')) {
      expect(url).not.toContain('//api/0');
      expect(url.startsWith('https://de.sentry.io/api/0/')).toBe(true);
    }
  });
});

describe('EXIT_REFUSE', () => {
  // It must not collide with anything else in the chain: node reserves 1-12 for its own
  // failures (1 uncaught throw / unresolvable module, 3 parse error, ...), sudo uses 1 when it
  // cannot run the command, and 128+ is the signal range. A collision would report a working
  // token as rejected — the confusion this whole check exists to end.
  it('cannot be produced by node, sudo, or a signal', () => {
    expect(EXIT_REFUSE).toBeGreaterThan(12);
    expect(EXIT_REFUSE).toBeLessThan(128);
  });
});

describe('configurationOf', () => {
  // This decides WHICH QUESTION gets asked, so a mistake here is invisible to verdict().
  const all = { SENTRY_ORG: 'o', SENTRY_PROJECT: 'p', SENTRY_AUTH_TOKEN: 't' };

  it('is complete when all three are set', () => {
    expect(configurationOf(all).configuration).toBe('complete');
  });

  it('is none when all three are absent', () => {
    expect(configurationOf({}).configuration).toBe('none');
  });

  it('is partial when some are set, and names the missing ones', () => {
    const got = configurationOf({ SENTRY_ORG: 'o', SENTRY_PROJECT: 'p' });
    expect(got.configuration).toBe('partial');
    expect(got.missing).toEqual(['SENTRY_AUTH_TOKEN']);
  });

  // Whitespace is how a hand-edited env file sets something to nothing.
  it('treats a blank value as unset', () => {
    expect(configurationOf({ ...all, SENTRY_AUTH_TOKEN: '   ' }).configuration).toBe('partial');
  });
});

describe('verdict', () => {
  it('treats no credential as a stated opt-out, not a failure', () => {
    const v = verdict({ configuration: 'none' });
    expect(v.ok).toBe(true);
    expect(v.message).toMatch(/minified/i);
  });

  // A truncated env file uploads nothing, exactly like an opt-out. If the two read alike, a
  // botched rotation ships green claiming it meant to.
  it('fails a half-written credential and names what is missing', () => {
    const v = verdict({ configuration: 'partial', missing: ['SENTRY_AUTH_TOKEN'] });
    expect(v.ok).toBe(false);
    expect(v.message).toMatch(/SENTRY_AUTH_TOKEN/);
  });

  it('passes a credential Sentry accepts, naming the org and project it accepted it for', () => {
    const v = verdict({ configuration: 'complete', status: 200, org: 'diffray', project: 'freehire-web' });
    expect(v.ok).toBe(true);
    expect(v.message).toMatch(/diffray\/freehire-web/);
  });

  // The production failure this change exists for: sentry-cli was answered
  // `Invalid token (http status: 401)` on every release and the build carried on regardless.
  it('fails a credential Sentry rejects, and says it was rejected', () => {
    const v = verdict({ configuration: 'complete', status: 401 });
    expect(v.ok).toBe(false);
    expect(v.message).toMatch(/rejected/i);
  });

  // design.md's Risks: a token that reads the org but cannot upload would sail through a
  // check that only asked "are you a token?".
  it('fails a valid credential that lacks the upload permission', () => {
    const v = verdict({ configuration: 'complete', status: 403 });
    expect(v.ok).toBe(false);
    expect(v.message).toMatch(/permission|scope/i);
  });

  // A 404 means the org or project slug is wrong, or the org is region-pinned — all things
  // somebody typed.
  it('fails when the org or project does not exist', () => {
    const v = verdict({ configuration: 'complete', status: 404 });
    expect(v.ok).toBe(false);
    expect(v.message).toMatch(/SENTRY_ORG|SENTRY_PROJECT/);
  });

  // The other half of the rule, and the one that keeps this check from becoming an outage:
  // a rejection proves a misconfiguration, an unreachable checker proves nothing. Blocking
  // every deploy on Sentry's uptime is the mistake cmd/llm-probe already records — a red
  // signal meaning "the thing I watch is broken" must not be confusable with "I am broken".
  it.each([[500], [502], [503], [418]])('does not stop the release on a %i', (status) => {
    const v = verdict({ configuration: 'complete', status });
    expect(v.ok).toBe(true);
    expect(v.message).toMatch(/could not verify/i);
  });

  it('does not stop the release when Sentry is unreachable', () => {
    const v = verdict({ configuration: 'complete', unreachable: 'getaddrinfo ENOTFOUND' });
    expect(v.ok).toBe(true);
    expect(v.message).toMatch(/could not verify/i);
  });

  // The probe always returns a status or an unreachable reason, so this is defensive — but it
  // must fall to the "cannot tell" leg rather than to a refusal, or a future outcome nobody
  // anticipated would start blocking deploys.
  it('does not stop the release on an outcome it cannot read', () => {
    const v = verdict({ configuration: 'complete' });
    expect(v.ok).toBe(true);
    expect(v.message).toMatch(/could not verify/i);
  });
});
