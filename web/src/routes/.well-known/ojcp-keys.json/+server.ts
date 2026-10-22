import { proxyDiscoveryDocument } from '#lib/server/wellKnown';
import type { RequestHandler } from './$types';

// The JWK Set the manifest's signature verifies against (spec § JWK Set Discovery). A
// deployment with no signing key publishes none, and this answers 404.
export const GET: RequestHandler = ({ fetch }) =>
  // "The JWK Set MUST be served over HTTPS with Content-Type: application/jwk-set+json."
  proxyDiscoveryDocument(fetch, '/api/v1/ojcp/keys', 'application/jwk-set+json');
