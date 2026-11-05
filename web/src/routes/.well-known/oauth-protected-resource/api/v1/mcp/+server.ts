import { proxyWellKnownJSON } from '#lib/server/wellKnownProxy';
import type { RequestHandler } from './$types';

// RFC 9728 discovery for the canonical MCP, independent of the legacy account URL.
export const GET: RequestHandler = ({ fetch }) =>
  proxyWellKnownJSON(fetch, '/api/v1/oauth/metadata/protected-resource/mcp');
