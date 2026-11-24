import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';

let pathname = '/matches';
const goto = vi.fn();
vi.mock('$app/navigation', () => ({ goto: (...a: unknown[]) => goto(...a) }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$app/state', () => ({
  page: {
    get url() {
      return new URL(`http://localhost${pathname}`);
    },
  },
}));

const { feedSession, signInAgainForFeed } = await import('./feedSession.svelte.js');

describe('feedSession', () => {
  beforeEach(() => {
    pathname = '/matches';
    feedSession.reset();
    goto.mockReset();
  });

  it('ends on a 401 only', () => {
    expect(feedSession.lost(new ApiError(500, 'x'))).toBe(false);
    expect(feedSession.ended).toBe(false);
    expect(feedSession.lost(new ApiError(401, 'x'))).toBe(true);
    expect(feedSession.ended).toBe(true);
  });

  it('ignores a late 401 once the visitor has left /matches', () => {
    pathname = '/jobs';
    expect(feedSession.lost(new ApiError(401, 'x'))).toBe(true); // still a 401: the caller shows no error
    expect(feedSession.ended).toBe(false);
  });

  it('asks for one sign-in navigation however many surfaces call', async () => {
    goto.mockResolvedValue(undefined);
    const a = signInAgainForFeed();
    const b = signInAgainForFeed();
    expect(a).toBe(b);
    await a;
    expect(goto).toHaveBeenCalledTimes(1);
    await signInAgainForFeed(); // settled: the next ask navigates again
    expect(goto).toHaveBeenCalledTimes(2);
  });
});
