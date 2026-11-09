// The shapes the API description is written in, and the constants its groups share.
// Kept apart from api-spec.ts so the group modules can import them without a cycle.

/** Production base URL for every path below. */
export const BASE_URL = 'https://freehire.me/api/v1';

/** Authentication requirement for an endpoint, rendered as a badge. */
export type Auth = 'none' | 'cookie-or-key' | 'cookie' | 'moderator' | 'extension';

/** Human-readable label for an auth level. */
export const AUTH_LABELS: Record<Auth, string> = {
  none: 'Public',
  'cookie-or-key': 'Session or API key',
  cookie: 'Session only',
  moderator: 'Moderator',
  extension: 'Browser extension only',
};

/** A single request parameter (path, query, or body field). */
export interface Param {
  name: string;
  type: string;
  required?: boolean;
  description: string;
  example?: string;
}

/** One HTTP endpoint. `curl` and `responseExample` are plain strings so they
 *  drop verbatim into both the page's code blocks and the Markdown fences. */
export interface Endpoint {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  path: string;
  auth: Auth;
  summary: string;
  description?: string;
  /** Marks the endpoint that carries the full job-search filter vocabulary, so
   *  the renderer can inject the shared filter table after its own params. */
  filterable?: boolean;
  /** Present when the endpoint is superseded but still served — both renderers
   *  mark it and name the replacement, rather than the caller learning only
   *  from prose. */
  deprecated?: { since: string; replacement: string };
  pathParams?: Param[];
  query?: Param[];
  body?: Param[];
  curl: string;
  responseExample?: string;
}

/** A group of related endpoints, rendered as one page section. The anchor is
 *  derived from the title (slugified) by both renderers, like Overview. */
export interface Group {
  title: string;
  intro: string;
  endpoints: Endpoint[];
}

/** A conceptual section before the endpoint reference (base URL, envelope,
 *  pagination, errors, auth model). Kept as paragraphs + an optional code
 *  block so neither renderer needs a Markdown parser. The anchor is derived
 *  from the title (slugified) by both renderers, so there is no separate id. */
export interface Overview {
  title: string;
  paragraphs: string[];
  code?: string;
}
