// The onboarding flow's draft: the visitor's CV, what was read from it and their answers,
// held in the browser only for the hop to the sign-in page and back.
//
// IndexedDB, not sessionStorage: the draft has to survive an OAuth round trip (a full page
// navigation to the provider and back, same origin) and hold a PDF of several megabytes as
// a Blob. Nothing about the CV is stored server-side before sign-up.
//
// It is written only when the visitor leaves for /signin from the results, and the page
// deletes it as soon as it reads it back — so an abandoned sign-up leaves nothing behind
// for the next person at the same browser, and a CV is never parked here for later. A
// draft older than a sign-up may take is ignored and deleted. Every operation is
// best-effort — private browsing may refuse IndexedDB, and the flow then runs without a
// draft (after the sign-in it asks for the CV again).

import { currentQuestion, type Answers } from './onboardingFlow';
import type { OnboardingParse } from './types';

export interface OnboardingDraft {
  file: Blob;
  fileName: string;
  parse: OnboardingParse;
  answers: Answers;
  /** Epoch milliseconds. */
  savedAt: number;
  /** When this visitor left the results for the sign-in page (epoch ms). The sign-in page
   *  may end in a registration or in a sign-in to an account that already existed, which
   *  `arrivalAction` tells apart; without this mark a draft reaches no account. */
  pendingSignupAt?: number;
}

/** How long a started sign-up may take — an OAuth round trip, or a registration. It is also
 *  the draft's whole life: nothing else writes one. */
export const PENDING_SIGNUP_MS = 30 * 60 * 1000;

export type ArrivalAction = 'restore' | 'carry' | 'offer-save' | 'discard';

/** What the page does with a draft it finds on arrival (and deletes right after).
 *
 *  Only a sign-up started from this browser moments ago counts; anything older or unmarked
 *  is discarded unseen. Signed out, the visitor came back from the sign-in page without an
 *  account and gets their results back. Signed in, the draft may still be somebody else's
 *  in a shared browser, so it reaches the account only when finished: carried at once into
 *  a new account, merely offered (an explicit save) to one that existed before. */
export function arrivalAction(
  draft: OnboardingDraft,
  ctx: { authenticated: boolean; accountCreatedAt: number | null; now: number },
): ArrivalAction {
  const marked = draft.pendingSignupAt;
  if (marked == null || ctx.now - marked > PENDING_SIGNUP_MS) return 'discard';
  if (!ctx.authenticated) return 'restore';
  if (currentQuestion(draft.answers) !== null) return 'discard';
  // New means created by this sign-up — after the visitor left for /signin. Not "never
  // onboarded": migration 0207 cleared that mark for existing accounts too.
  const createdBySignup = ctx.accountCreatedAt != null && ctx.accountCreatedAt >= marked - CLOCK_SKEW_MS;
  return createdBySignup ? 'carry' : 'offer-save';
}

/** How far the browser's clock may run ahead of the server's when comparing the sign-up
 *  mark (browser time) with the account's creation (server time). */
const CLOCK_SKEW_MS = 2 * 60 * 1000;

/** The one-record store the draft lives in. */
export interface DraftBackend {
  get(): Promise<unknown>;
  put(value: unknown): Promise<void>;
  delete(): Promise<void>;
}

function isDraft(v: unknown): v is OnboardingDraft {
  if (!v || typeof v !== 'object') return false;
  const d = v as Record<string, unknown>;
  return d.file instanceof Blob && typeof d.savedAt === 'number' && typeof d.parse === 'object' && d.parse !== null;
}

export function createDraftStore(open: () => Promise<DraftBackend | null>) {
  // One operation at a time, in the order asked: a clear issued while a save is still
  // writing must land after it, or the late save brings the draft back.
  let queue: Promise<unknown> = Promise.resolve();

  function attempt<T>(fn: (b: DraftBackend) => Promise<T>, fallback: T): Promise<T> {
    const run = queue.then(async () => {
      try {
        const backend = await open();
        return backend ? await fn(backend) : fallback;
      } catch {
        return fallback;
      }
    });
    queue = run;
    return run;
  }

  return {
    save(draft: OnboardingDraft): Promise<void> {
      return attempt((b) => b.put(draft), undefined);
    },
    /** The live draft, or null — an expired one is deleted on the way. */
    load(now = Date.now()): Promise<OnboardingDraft | null> {
      return attempt(async (b) => {
        const v = await b.get();
        if (!isDraft(v)) return null;
        if (now - v.savedAt > PENDING_SIGNUP_MS) {
          await b.delete();
          return null;
        }
        return v;
      }, null);
    },
    clear(): Promise<void> {
      return attempt((b) => b.delete(), undefined);
    },
  };
}

const DB_NAME = 'freehire-onboarding';
const STORE = 'draft';
const KEY = 'current';

function done<T>(req: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    req.addEventListener('success', () => resolve(req.result));
    req.addEventListener('error', () => reject(req.error));
  });
}

let db: Promise<IDBDatabase> | null = null;

function openIndexedDb(): Promise<DraftBackend | null> {
  if (typeof indexedDB === 'undefined') return Promise.resolve(null);
  if (!db) {
    const req = indexedDB.open(DB_NAME, 1);
    req.addEventListener('upgradeneeded', () => req.result.createObjectStore(STORE));
    db = done(req);
    // A refused open must not be cached, or the next attempt never retries.
    db.catch(() => (db = null));
  }
  return db.then((conn) => {
    const store = (mode: IDBTransactionMode) => conn.transaction(STORE, mode).objectStore(STORE);
    // A write counts once its transaction commits, not when the request succeeds: the
    // sign-up leaves the page right after the draft is saved.
    const write = (op: (s: IDBObjectStore) => IDBRequest) => {
      const tx = conn.transaction(STORE, 'readwrite');
      op(tx.objectStore(STORE));
      return new Promise<void>((resolve, reject) => {
        tx.addEventListener('complete', () => resolve());
        tx.addEventListener('error', () => reject(tx.error));
        tx.addEventListener('abort', () => reject(tx.error));
      });
    };
    return {
      get: () => done(store('readonly').get(KEY)),
      put: (value: unknown) => write((s) => s.put(value, KEY)),
      delete: () => write((s) => s.delete(KEY)),
    };
  });
}

export const onboardingDraft = createDraftStore(openIndexedDb);
