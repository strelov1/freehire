// Catching an apply conversation up on its application: the server narrates whatever
// happened to the application since the transcript was last read.

import { StreamInterrupted, TurnRefused, syncApplication, type Turn } from '#lib/assistant/client.js';
import type { TurnEvent } from '#lib/assistant/wire.js';

// How often an open apply conversation asks whether its application moved. The worker that
// moves it runs every five minutes, so this only has to be prompt, not instant.
const SYNC_INTERVAL_MS = 15_000;

/** What the chat does with a check that became a turn. */
export interface ApplicationSyncHost {
  /** A turn is streaming or held, so no check may start. */
  busy(): boolean;
  /** The server started narrating: the check is a turn now. */
  began(turn: Turn): void;
  event(sessionId: string, event: TurnEvent): void;
  /** Re-read the stored transcript after a broken stream. */
  reload(sessionId: string): Promise<void>;
  /** The check that became a turn finished. */
  ended(): void;
}

export class ApplicationSync {
  // A check still waiting on the server, before it has painted anything. Without it a slow
  // first token lets the next tick send a second check for the same events.
  #catchingUp = false;
  // The plan refused the check: every later one would be refused the same way.
  #refused = false;

  readonly #host: ApplicationSyncHost;

  constructor(host: ApplicationSyncHost) {
    this.#host = host;
  }

  /** Check now and on every interval; returns the cleanup for the caller's effect. */
  watch(sessionId: string): () => void {
    const check = () => {
      if (!this.#host.busy() && !this.#catchingUp && !this.#refused) void this.#catchUp(sessionId);
    };
    check();
    const timer = setInterval(check, SYNC_INTERVAL_MS);
    return () => clearInterval(timer);
  }

  /** One catch-up. It only becomes a turn once the server starts narrating: a check with
   *  nothing new must not flash the "thinking" state every fifteen seconds. A failed check is
   *  left for the next one — it is not something the candidate asked for or can act on. */
  async #catchUp(sessionId: string) {
    this.#catchingUp = true;
    let began = false;
    const started = syncApplication(sessionId, (event) => {
      if (!began) {
        began = true;
        this.#host.began(started);
      }
      this.#host.event(sessionId, event);
    });
    try {
      await started.done;
    } catch (err) {
      if (err instanceof StreamInterrupted) await this.#host.reload(sessionId);
      if (err instanceof TurnRefused) this.#refused = true;
    } finally {
      this.#catchingUp = false;
    }
    if (began) this.#host.ended();
  }
}
