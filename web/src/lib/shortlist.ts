import type { SessionSummary } from './assistant/wire';
import type { SessionTranscript } from './assistant/api';

/** The current filters, without pagination belonging to the list rather than the search. */
export function shortlistQuery(params: URLSearchParams | string): string {
  const snapshot = new URLSearchParams(params);
  for (const key of ['page', 'offset', 'limit']) snapshot.delete(key);
  snapshot.sort();
  return snapshot.toString();
}

type Attempt = { requestId: string; query: string; sessionId?: string };
type Create = (query: string, requestId: string) => Promise<SessionSummary>;
type Read = (id: string) => Promise<SessionTranscript>;
const PREFIX = 'hire.shortlist:';
const UUID = /^\p{AHex}{8}(?:-\p{AHex}{4}){3}-\p{AHex}{12}$/u;

function restore(userId: number): Attempt | null {
  try {
    const raw = sessionStorage.getItem(PREFIX + userId);
    if (!raw) return null;
    const value: unknown = JSON.parse(raw);
    if (typeof value !== 'object' || value === null) return null;
    const attempt = value as Partial<Attempt>;
    if (typeof attempt.query !== 'string' || typeof attempt.requestId !== 'string' || !UUID.test(attempt.requestId)) return null;
    if (attempt.sessionId !== undefined && (typeof attempt.sessionId !== 'string' || !UUID.test(attempt.sessionId))) return null;
    return { requestId: attempt.requestId, query: attempt.query, sessionId: attempt.sessionId };
  } catch {
    return null;
  }
}

/** One tab's search conversation for one account. Creation retries retain their UUID. */
export class ShortlistSession {
  #attempt: Attempt | null;
  #session: SessionSummary | null = null;
  #opening: Promise<SessionSummary> | null = null;
  #userId: number;

  constructor(userId: number) {
    this.#userId = userId;
    this.#attempt = restore(userId);
  }

  get query(): string | undefined {
    return this.#attempt?.query;
  }

  #persist() {
    try {
      if (this.#attempt) sessionStorage.setItem(PREFIX + this.#userId, JSON.stringify(this.#attempt));
      else sessionStorage.removeItem(PREFIX + this.#userId);
    } catch {
      // Private mode and disabled storage must not prevent an interactive conversation.
    }
  }

  open(query: string, create: Create, read: Read): Promise<SessionSummary> {
    if (this.#opening) return this.#opening;
    if (this.#session) return Promise.resolve(this.#session);
    this.#attempt ??= { requestId: crypto.randomUUID(), query };
    this.#persist();
    const attempt = this.#attempt;
    this.#opening = (attempt.sessionId
      ? read(attempt.sessionId).then((result) => result.session)
      : create(attempt.query, attempt.requestId)
    ).then((session) => {
      if (!session.search_context) throw new Error('This conversation has no saved search.');
      this.#session = session;
      this.#attempt = { ...attempt, query: session.search_context.query, sessionId: session.id };
      this.#persist();
      return session;
    }).finally(() => { this.#opening = null; });
    return this.#opening;
  }

  startNew(): void {
    if (this.#opening) throw new Error('The conversation is still opening.');
    this.#attempt = null;
    this.#session = null;
    this.#persist();
  }
}
