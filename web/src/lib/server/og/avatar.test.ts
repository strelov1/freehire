import { describe, expect, it } from 'vitest';
import { resolveAvatar } from './avatar';

describe('resolveAvatar', () => {
  // The URL comes from a snapshot we commit, so it is trusted — but this function
  // turns a URL into a server-side fetch, and that is the shape of request worth
  // bounding at the door rather than trusting by provenance. A host that is not
  // GitHub's avatar CDN is refused before any request leaves.
  it('refuses a host that is not the GitHub avatar CDN', async () => {
    expect(await resolveAvatar('http://169.254.169.254/latest/meta-data/')).toBeNull();
    expect(await resolveAvatar('https://example.com/avatar.png')).toBeNull();
  });

  it('refuses a lookalike host', async () => {
    expect(await resolveAvatar('https://avatars.githubusercontent.com.evil.test/x')).toBeNull();
  });

  it('refuses something that is not a URL at all', async () => {
    expect(await resolveAvatar('not a url')).toBeNull();
  });
});
