import type { TalentNetworkVisibility } from '$lib/types';

// Whether a candidate is IN the Talent Network, asked once.
//
// Three surfaces decide this — the membership toggle, the invitation card on the
// profile, and the join button on the catalogue — and each of them spelled the
// comparison itself. That is one concept written three times, and the enum behind it
// has already changed once: `public` was retired with the mode picker (migration 0148),
// which is exactly the kind of move that leaves one of three copies behind.
//
// It takes the visibility rather than the whole `TalentNetworkSetting` because two of
// the three callers hold only that, having read the setting into local state.
//
// It is NOT the same question as "can a visitor see them" — that is `setting.listed`,
// which a member without a finished CV fails. See TalentNetworkSetting's own comment.

/** True when the candidate is a Talent Network member. */
export function isTalentNetworkMember(visibility: TalentNetworkVisibility): boolean {
  return visibility !== 'off';
}
