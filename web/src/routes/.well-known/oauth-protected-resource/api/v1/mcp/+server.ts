import { proxyWellKnownJSON } from '#lib/server/wellKnownProxy';
import type { RequestHandler } from './$types';

// RFC 9728's path-inserted form for /api/v1/mcp, which clients following the
// 2025-11-25 discovery order ask for before the root document. Same resource,
// same document.
export const GET: RequestHandler = ({ fetch }) =>
  proxyWellKnownJSON(fetch, '/api/v1/oauth/metadata/protected-resource');
