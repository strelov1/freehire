import { describe, expect, it } from 'vitest';
import { isRedirect } from '@sveltejs/kit';
import { load as listLoad } from './+page.server';
import { load as memberLoad } from './[handle]/+page.server';

// Emails and notifications already sent carry the cabinet addresses; each must land on the
// same thing in the buddies space.
function target(run: () => unknown): { status: number; location: string } {
  try {
    run();
  } catch (e) {
    if (isRedirect(e)) return { status: e.status, location: e.location };
    throw e;
  }
  throw new Error('expected a redirect');
}

describe('/my/buddies redirects', () => {
  it('sends an unread notice link to that conversation in Chats', () => {
    const url = new URL('https://freehire.me/my/buddies?c=42');
    expect(target(() => listLoad({ url } as never))).toEqual({ status: 308, location: '/buddies/chats?c=42' });
  });

  it('sends the bare address to the feed', () => {
    const url = new URL('https://freehire.me/my/buddies');
    expect(target(() => listLoad({ url } as never))).toEqual({ status: 308, location: '/buddies' });
  });

  it('keeps any other query on the feed redirect', () => {
    const url = new URL('https://freehire.me/my/buddies?utm_source=email');
    expect(target(() => listLoad({ url } as never))).toEqual({ status: 308, location: '/buddies?utm_source=email' });
  });

  it('sends a member page to the same member, query kept', () => {
    const url = new URL('https://freehire.me/my/buddies/amber-heron-42?utm_source=email');
    expect(target(() => memberLoad({ params: { handle: 'amber-heron-42' }, url } as never))).toEqual({
      status: 308,
      location: '/buddies/amber-heron-42?utm_source=email',
    });
  });
});
