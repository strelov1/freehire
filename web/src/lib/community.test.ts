import { beforeEach, describe, expect, it, vi } from 'vitest';

const { promptLogin, promptSignIn } = vi.hoisted(() => ({ promptLogin: vi.fn(), promptSignIn: vi.fn() }));
vi.mock('./signin', () => ({ promptLogin, promptSignIn }));

import { ApiError } from './api';
import { communityFormError } from './community';

describe('communityFormError', () => {
  beforeEach(() => vi.clearAllMocks());

  it('sends a lapsed session straight to login, not to onboarding (the account still looks signed in)', () => {
    expect(communityFormError(new ApiError(401, 'unauthorized'))).toBeNull();
    expect(promptLogin).toHaveBeenCalledOnce();
    expect(promptSignIn).not.toHaveBeenCalled();
  });
});
