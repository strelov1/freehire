/**
 * Which page a `read_current_page` call actually read, for the transcript.
 *
 * A read the user cannot see is a read they cannot object to, and the panel is
 * where they would. The tool takes no arguments, so the url exists only in what
 * came back — this reads it out of the result.
 *
 * Deliberately NOT in `tool-formatters.ts`: that file is a verbatim port of the web
 * app's, and its worth is that it stays one. `read_current_page` cannot occur in
 * the web app, so its display belongs beside the panel that renders it.
 */

import type { ToolCall } from './tool-formatters';

/**
 * The host and path of the page a call read — `boards.greenhouse.io/acme/jobs/12`.
 *
 * The query string and the fragment are dropped. Both routinely carry one-time
 * tokens and session identifiers, and this line is written into a transcript that
 * is replayed on every later turn.
 *
 * Returns an empty string for anything it cannot read a url out of, including a
 * failed call: the caller then shows its plain label, because the read did happen
 * and saying so imprecisely beats saying nothing.
 */
export function pageReadTarget(call: ToolCall): string {
  if (call.isError || !call.result) return '';
  try {
    const { url } = JSON.parse(call.result) as { url?: unknown };
    if (typeof url !== 'string') return '';
    const { host, pathname } = new URL(url);
    // A root path renders as the bare host: "example.test", not "example.test/".
    return pathname === '/' ? host : host + pathname;
  } catch {
    // Unparseable JSON and an unparseable url are the same outcome here — there is
    // no page to name — so they share one exit rather than two identical ones.
    return '';
  }
}
