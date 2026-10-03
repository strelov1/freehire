import { proxyWellKnownJSON } from '$lib/server/wellKnownProxy';
import type { RequestHandler } from './$types';

// RFC 9728's protected-resource metadata: which authorization server protects
// the signed-in MCP server at /api/v1/mcp/account. See
// oauth-authorization-server/+server.ts beside it for why this proxies.
export const GET: RequestHandler = ({ fetch }) =>
  proxyWellKnownJSON(fetch, '/api/v1/oauth/metadata/protected-resource');
