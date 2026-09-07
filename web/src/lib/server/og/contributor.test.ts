import { describe, expect, it } from 'vitest';
import { buildContributorCard } from './contributor';
import type { ContributorEntry } from '$lib/contributors';

function person(over: Partial<ContributorEntry> = {}): ContributorEntry {
  return {
    login: 'aleganza',
    id: 1,
    avatarUrl: 'https://avatars.githubusercontent.com/u/1',
    accountType: 'User',
    role: 'contributor',
    firstContributionAt: '2026-07-15T00:00:00Z',
    lastContributionAt: '2026-08-30T00:00:00Z',
    mergedPullRequests: 8,
    openedIssues: 3,
    recentPullRequests: [],
    ...over,
  };
}

describe('buildContributorCard', () => {
  it('names the contributor and what they contributed', () => {
    const markup = buildContributorCard(person(), { avatar: null });

    expect(markup).toContain('aleganza');
    expect(markup).toContain('8 merged pull requests · 3 issues opened');
  });

  it('says when they first contributed', () => {
    const markup = buildContributorCard(person(), { avatar: null });

    expect(markup).toContain('July 2026');
  });

  it('embeds the avatar when one was resolved', () => {
    const markup = buildContributorCard(person(), { avatar: 'data:image/png;base64,AAA' });

    expect(markup).toContain('data:image/png;base64,AAA');
  });

  // satori cannot fetch a remote image, so the endpoint resolves the avatar to a
  // data-URI first. When that fetch fails the card must still render — a contributor
  // who deleted their account, or a slow CDN, cannot be allowed to fail the image.
  it('falls back to a monogram when no avatar was resolved', () => {
    const markup = buildContributorCard(person({ login: 'aleganza' }), { avatar: null });

    expect(markup).not.toContain('<img src="https://');
    expect(markup).toContain('AL');
  });

  // The card is built by string concatenation, so anything interpolated into it can
  // otherwise close a tag. A GitHub login cannot contain these characters today, but
  // the escaping is what makes that not matter.
  it('escapes anything interpolated into the markup', () => {
    const markup = buildContributorCard(person({ login: '<script>x</script>' }), { avatar: null });

    expect(markup).not.toContain('<script>');
    expect(markup).toContain('&lt;script&gt;');
  });

  // A person is drawn in a circle and an entity in a rounded tile — the same
  // distinction the design system's Avatar makes with its `shape` prop. A card about a
  // person that squares them off reads as a company logo.
  it('draws the person in a circle, not a company tile', () => {
    const withPhoto = buildContributorCard(person(), { avatar: 'data:image/png;base64,AAA' });
    const withMonogram = buildContributorCard(person(), { avatar: null });

    expect(withPhoto).toContain('border-radius:50%');
    expect(withMonogram).toContain('border-radius:50%');
  });

  it('calls a maintainer one', () => {
    const markup = buildContributorCard(person({ role: 'maintainer' }), { avatar: null });

    expect(markup.toLowerCase()).toContain('maintain');
  });

  // Every element with more than one child has to declare `display:flex` or satori
  // throws at render time — a constraint the render smoke test catches, but only after
  // a full font load and rasterisation.
  it('closes with the shared brand footer', () => {
    const markup = buildContributorCard(person(), { avatar: null });

    expect(markup).toContain('freehire.me');
  });
});
