import { error } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import { rethrowUpstream } from '$lib/server/upstream';
import type { PageServerLoad } from './$types';

// The public, unauthenticated Talent Network profile page: load one candidate's profile
// by their opaque public id. "off" (hidden) and "no such id" answer with the identical
// 404 from the backend (internal/handler/talent_network_profile.go) — this load does not
// try to tell them apart, matching the spec's "identical response shape" requirement.
// Public read: no cookie forwarded.
export const load: PageServerLoad = async ({ params, fetch }) => {
  const client = serverApi(fetch);

  try {
    const profile = await client.getTalentNetworkProfile(params.publicId);
    return { profile };
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) error(404, 'Profile not found');
    rethrowUpstream(e);
  }
};
