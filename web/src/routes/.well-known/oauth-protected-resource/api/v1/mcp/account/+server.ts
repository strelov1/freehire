import { proxyWellKnownJSON } from '#lib/server/wellKnownProxy';
import type { RequestHandler } from './$types';

// RFC 9728's path-inserted form for /api/v1/mcp/account, the saved URL of connectors added
// before #4539. Its own document: `resource` must equal the URL the client derived this from.
export const GET: RequestHandler = ({ fetch }) =>
  proxyWellKnownJSON(fetch, '/api/v1/oauth/metadata/protected-resource/account');
