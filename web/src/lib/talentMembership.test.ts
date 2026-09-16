import { describe, it, expect } from 'vitest';
import { isTalentNetworkMember } from './talentMembership';

describe('isTalentNetworkMember', () => {
  it('reads the two live values', () => {
    expect(isTalentNetworkMember('off')).toBe(false);
    expect(isTalentNetworkMember('anonymous')).toBe(true);
  });

  // The enum has already lost a value once (`public`, migration 0148) and could gain one.
  // Membership is "anything but off", never a list of the modes that count — a list would
  // silently answer false for a mode added later, quietly un-joining whoever picked it.
  it('treats an unrecognised mode as membership, not as absence', () => {
    expect(isTalentNetworkMember('public' as never)).toBe(true);
  });
});
