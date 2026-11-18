import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { User } from '#lib/types.js';
import { adoptBrowserTimezone } from './auth.svelte';

// An OAuth sign-up carries no timezone, so the account stays UTC until something stores
// the browser's. These tests pin that only an account with none is written, and that a
// failed write never escapes into the layout that fires it.

const { updateTimezone, invalidateAll } = vi.hoisted(() => ({
  updateTimezone: vi.fn(),
  invalidateAll: vi.fn(),
}));

vi.mock('#lib/api.js', () => ({ api: { updateTimezone } }));
vi.mock('$app/navigation', () => ({ invalidateAll }));
vi.mock('$app/state', () => ({ page: { data: {} } }));

const user = (timezone: string | null) => ({ id: 1, email: 'a@b.c', timezone }) as User;

beforeEach(() => {
  vi.spyOn(Intl, 'DateTimeFormat').mockReturnValue({
    resolvedOptions: () => ({ timeZone: 'Asia/Karachi' }),
  } as Intl.DateTimeFormat);
});

afterEach(() => {
  vi.restoreAllMocks();
  updateTimezone.mockReset();
  invalidateAll.mockReset();
});

describe('adoptBrowserTimezone', () => {
  it('stores the browser zone on an account that has none', async () => {
    await adoptBrowserTimezone(user(null));
    expect(updateTimezone).toHaveBeenCalledWith('Asia/Karachi', true);
    expect(invalidateAll).toHaveBeenCalled();
  });

  it('leaves a stored zone alone', async () => {
    await adoptBrowserTimezone(user('Europe/Berlin'));
    expect(updateTimezone).not.toHaveBeenCalled();
  });

  it('swallows a failed write', async () => {
    updateTimezone.mockRejectedValue(new Error('400'));
    await expect(adoptBrowserTimezone(user(null))).resolves.toBeUndefined();
  });

  it('does not write when timezone detection fails', async () => {
    vi.mocked(Intl.DateTimeFormat).mockImplementation(() => {
      throw new Error('unavailable');
    });
    await expect(adoptBrowserTimezone(user(null))).resolves.toBeUndefined();
    expect(updateTimezone).not.toHaveBeenCalled();
  });
  it('does not write an empty detected zone', async () => {
    vi.mocked(Intl.DateTimeFormat).mockReturnValue({
      resolvedOptions: () => ({ timeZone: '' }),
    } as Intl.DateTimeFormat);
    await adoptBrowserTimezone(user(null));
    expect(updateTimezone).not.toHaveBeenCalled();
  });
});
