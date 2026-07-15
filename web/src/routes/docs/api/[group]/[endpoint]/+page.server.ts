// Server-only load for a single endpoint page: resolve the (group, endpoint)
// slug to its api-spec entry and Shiki-highlight its request/response. Kept in
// `+page.server.ts` so Shiki stays out of the client bundle.
import { error } from '@sveltejs/kit';
import { findEndpoint } from '$lib/docs/nav';
import { highlight, langFor } from '$lib/docs/highlight';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ params }) => {
  const found = findEndpoint(params.group, params.endpoint);
  if (!found) throw error(404, 'Unknown API endpoint');

  const ep = found.entry.endpoint;
  const isStream = !!ep.responseExample && ep.responseExample.trimStart().startsWith('data:');
  const curlHtml = await highlight(ep.curl, 'bash');
  const responseHtml = ep.responseExample
    ? await highlight(ep.responseExample, langFor('json', ep.responseExample))
    : null;

  return {
    groupTitle: found.group.title,
    endpoint: ep,
    curlHtml,
    responseHtml,
    responseLabel: isStream ? 'text/event-stream' : 'json — response',
  };
};
