import { redirect } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

// The short form of an invite link: `freehire.me/r/<code>`.
//
// It carries no capture logic of its own. The request hook in hooks.server.ts already
// stores a `ref` parameter arriving on ANY url, so this route's whole job is to turn a
// short path into that parameter and send the visitor to the front page. Doing the capture
// twice would be two rules to keep in step, and the one that drifted would be this one —
// it is the path nobody tests by accident.
//
// A GET redirect, so it works from a chat client's link preview, an email, or a QR code
// without any script running.
export const GET: RequestHandler = ({ params, url }) => {
  const target = new URL('/', url.origin);
  target.searchParams.set('ref', params.code);
  // 302 and not 301: a permanent redirect is cached by the browser forever, and this path
  // must keep reaching the server so the cookie is set again on a device that lost it.
  redirect(302, target.pathname + target.search);
};
