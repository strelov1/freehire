import { proxyDiscoveryDocument } from '#lib/server/wellKnown';
import type { RequestHandler } from './$types';

// The OJCP manifest — the one document an OJCP provider MUST serve, and how an agent
// discovers everything else. The Go service renders it from its own configuration (`tools`
// from what the MCP server registered, `rate_limits` from the constant the limiter
// enforces), because the spec makes both claims binding.
export const GET: RequestHandler = ({ fetch }) =>
  // The spec requires this content type by name.
  proxyDiscoveryDocument(fetch, '/api/v1/ojcp/manifest', 'application/json; charset=utf-8');
