import { contributorGroups } from '$lib/contributors';
import type { ContributorsSnapshot } from '$lib/contributors';
import snapshot from '$lib/data/contributors.json';
import type { PageServerLoad } from './$types';

// The showcase reads a snapshot committed to the repository, never the GitHub API. The
// site already spends GitHub's unauthenticated 60-requests-per-hour-per-IP budget on the
// header's star badge and /open (see $lib/server/github), and per-person pull-request
// lists do not fit inside what is left. A daily workflow collects the file instead, so
// this page cannot rate-limit, cannot fail, and costs nothing to serve.
//
// Grouping and ordering are not done here — $lib/contributors owns them and is tested.
export const load: PageServerLoad = () => contributorGroups(snapshot as ContributorsSnapshot);
