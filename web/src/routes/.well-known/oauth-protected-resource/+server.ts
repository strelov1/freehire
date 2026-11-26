import { proxyDiscoveryDocument } from '#lib/server/wellKnown';
import type { RequestHandler } from './$types';

// RFC 9728's protected-resource metadata: which authorization server protects
// the MCP server at /api/v1/mcp. See oauth-authorization-server/+server.ts
// beside it for why this proxies.
export const GET: RequestHandler = ({ fetch }) =>
  proxyDiscoveryDocument(fetch, '/api/v1/oauth/metadata/protected-resource', 'application/json; charset=utf-8');
