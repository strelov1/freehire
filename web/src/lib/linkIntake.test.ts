import { describe, it, expect, vi } from 'vitest';
import { runLinkIntake, type LinkIntakeDeps } from './linkIntake';
import type { ResolvedLink } from './types';

function deps(over: Partial<LinkIntakeDeps> = {}): LinkIntakeDeps {
  return {
    find: vi.fn(async () => null),
    submit: vi.fn(async () => ({ public_slug: null, status: 'queued' }) as ResolvedLink),
    signedIn: () => true,
    ...over,
  };
}

describe('runLinkIntake', () => {
  it('opens a posting the catalog already carries, without submitting it', async () => {
    const d = deps({ find: vi.fn(async () => ({ public_slug: 'go-dev-acme' })) });
    await expect(runLinkIntake('https://acme.com/jobs/1', d)).resolves.toEqual({
      kind: 'open',
      slug: 'go-dev-acme',
    });
    expect(d.submit).not.toHaveBeenCalled();
  });

  it('opens it for a signed-out visitor too — the lookup is the public half', async () => {
    const d = deps({
      find: vi.fn(async () => ({ public_slug: 'go-dev-acme' })),
      signedIn: () => false,
    });
    await expect(runLinkIntake('https://acme.com/jobs/1', d)).resolves.toEqual({
      kind: 'open',
      slug: 'go-dev-acme',
    });
    expect(d.submit).not.toHaveBeenCalled();
  });

  it('asks a signed-out visitor to sign in only once the link turns out to be new', async () => {
    const d = deps({ signedIn: () => false });
    await expect(runLinkIntake('https://acme.com/jobs/1', d)).resolves.toEqual({ kind: 'signin' });
    expect(d.submit).not.toHaveBeenCalled();
  });

  it('submits an unknown link for a signed-in visitor and reports the outcome', async () => {
    const resolved: ResolvedLink = { public_slug: null, status: 'queued' };
    const d = deps({ submit: vi.fn(async () => resolved) });
    await expect(runLinkIntake('https://acme.com/jobs/1', d)).resolves.toEqual({
      kind: 'outcome',
      resolved,
    });
  });

  it('opens the posting the intake imported rather than narrating the import', async () => {
    const d = deps({
      submit: vi.fn(async () => ({ public_slug: 'go-dev-acme', status: 'imported' }) as ResolvedLink),
    });
    await expect(runLinkIntake('https://acme.com/jobs/1', d)).resolves.toEqual({
      kind: 'open',
      slug: 'go-dev-acme',
    });
  });

  it('falls through to the intake when the public lookup fails', async () => {
    const d = deps({
      find: vi.fn(async () => {
        throw new Error('502');
      }),
      submit: vi.fn(async () => ({ public_slug: 'go-dev-acme', status: 'found' }) as ResolvedLink),
    });
    await expect(runLinkIntake('https://acme.com/jobs/1', d)).resolves.toEqual({
      kind: 'open',
      slug: 'go-dev-acme',
    });
  });

  it('gives a signed-out visitor an error when the lookup fails — there is no second door for them', async () => {
    const d = deps({
      find: vi.fn(async () => {
        throw new Error('502');
      }),
      signedIn: () => false,
    });
    await expect(runLinkIntake('https://acme.com/jobs/1', d)).resolves.toEqual({ kind: 'error' });
  });

  it('reports an intake failure as an error rather than a verdict on the link', async () => {
    const d = deps({
      submit: vi.fn(async () => {
        throw new Error('422');
      }),
    });
    await expect(runLinkIntake('not-a-url', d)).resolves.toEqual({ kind: 'error' });
  });
});
