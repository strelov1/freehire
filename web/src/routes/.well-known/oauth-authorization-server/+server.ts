import { proxyWellKnownJSON } from '$lib/server/wellKnownProxy';
import type { RequestHandler } from './$types';

// RFC 8414's authorization-server metadata. An MCP client (Claude, Cursor,
// ChatGPT) fetches this before ever calling /authorize, to find the
// registration/authorize/token endpoints without them being hardcoded.
// See wellKnownProxy.ts for why this proxies rather than being a Go route.
export const GET: RequestHandler = ({ fetch }) =>
  proxyWellKnownJSON(fetch, '/api/v1/oauth/metadata/authorization-server');
