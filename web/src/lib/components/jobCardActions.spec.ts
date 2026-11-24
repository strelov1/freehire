import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/api-core.js';

const dismissJob = vi.fn();
const saveJob = vi.fn();
vi.mock('#lib/api.js', () => ({
  api: { dismissJob: (s: string) => dismissJob(s), saveJob: (s: string) => saveJob(s) },
}));
let userID = 'u1';
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => true, currentUser: () => ({ id: userID }) }));
vi.mock('#lib/signin.js', () => ({ promptSignIn: vi.fn() }));
const markUndismissed = vi.fn();
vi.mock('#lib/dismissedJobs.svelte.js', () => ({
  markDismissed: vi.fn(),
  markUndismissed: (s: string) => markUndismissed(s),
}));
vi.mock('#lib/savedJobs.svelte.js', () => ({
  ensureSavedLoaded: vi.fn(),
  isSaved: () => false,
  markSaved: vi.fn(),
  markUnsaved: vi.fn(),
}));

const { JobCardActions } = await import('./jobCardActions.svelte.js');
const make = (onUnauthorized?: () => void) =>
  new JobCardActions(
    () => 'a',
    () => async () => {},
    () => undefined,
    onUnauthorized && (() => onUnauthorized),
  );

describe('JobCardActions session loss', () => {
  beforeEach(() => {
    userID = 'u1';
    dismissJob.mockReset();
    saveJob.mockReset();
  });

  it.each([
    { action: 'hide', request: dismissJob },
    { action: 'toggleSave', request: saveJob },
  ] as const)('ignores a delayed 401 from $action after an account switch', async ({ action, request }) => {
    const lost = vi.fn();
    const write = Promise.withResolvers<undefined>();
    request.mockReturnValueOnce(write.promise);
    const pending = make(lost)[action]();
    await vi.waitFor(() => expect(request).toHaveBeenCalledWith('a'));

    userID = 'u2';
    write.reject(new ApiError(401, 'unauthorized'));
    await pending;

    expect(lost).not.toHaveBeenCalled();
  });

  it('tells an opted-in surface about a 401 on hide and on save, after the usual rollback', async () => {
    const lost = vi.fn();
    dismissJob.mockRejectedValue(new ApiError(401, 'unauthorized'));
    saveJob.mockRejectedValue(new ApiError(401, 'unauthorized'));
    const actions = make(lost);
    await actions.hide();
    await actions.toggleSave();
    expect(markUndismissed).toHaveBeenCalledWith('a');
    expect(lost).toHaveBeenCalledTimes(2);
  });

  it('keeps the silent rollback for every other surface and for other failures', async () => {
    const lost = vi.fn();
    dismissJob.mockRejectedValue(new ApiError(401, 'unauthorized'));
    await make().hide();
    dismissJob.mockRejectedValue(new ApiError(500, 'boom'));
    await make(lost).hide();
    expect(lost).not.toHaveBeenCalled();
  });
});
