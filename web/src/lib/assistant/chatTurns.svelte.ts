import { SUPPORT_BASE } from '#lib/assistant/api.js';
import { turnBegins } from '#lib/assistant/chat.js';
import { StreamInterrupted, TurnRefused, startTurnFor, type Turn, type TurnStart } from '#lib/assistant/client.js';
import type { TurnEvent } from '#lib/assistant/wire.js';
import type { ChatAlerts } from './chatAlerts.svelte';

/** What a turn needs from the chat around it. Getters, so a prop the host changes is read live. */
export interface ChatTurnsHost {
  isSupport: boolean;
  alerts: ChatAlerts;
  activeId(): string | null;
  disposed(): boolean;
  text(): { messageNotSent: string; supportInterrupted: string; sendFailed: string };
  /** A streamed frame for session `id`. */
  event(id: string, event: TurnEvent): void;
  /** Fold a frame the client made up into the open transcript. */
  reduce(event: TurnEvent): void;
  reload(id: string): Promise<void>;
  /** Return an unsent message to the composer. */
  giveBack(start: TurnStart): void;
  /** The turn is over (see AssistantChat's endTurn). */
  ended(): void;
  followBottom(): void;
  beforeTurn(): (() => Promise<void>) | undefined;
  onTurnStateChange(): ((active: boolean) => void) | undefined;
  onRunStateChange(): ((running: boolean) => void) | undefined;
}

/**
 * The open conversation's turn: whether one is streaming, whether it is an unattended run,
 * and the in-flight request, so it can be cancelled. Cancelling asks the server to stop the
 * work; aborting the fetch only stops this client reading it.
 */
export class ChatTurns {
  active = $state(false);
  // Whether the turn in flight is an unattended run rather than a reply to something typed.
  runActive = $state(false);
  #turn: Turn | null = null;

  constructor(private readonly host: ChatTurnsHost) {}

  /** A turn is in flight or claimed. */
  get busy() {
    return this.active || this.#turn !== null;
  }

  /** A turn started outside `run` (application sync) is now this chat's. */
  adopt(started: Turn) {
    this.active = true;
    this.#turn = started;
    this.host.onTurnStateChange()?.(true);
  }

  /** The turn ended: clear it and tell the host. */
  finish() {
    this.active = false;
    this.#turn = null;
    this.host.onTurnStateChange()?.(false);
    if (this.runActive) {
      this.runActive = false;
      this.host.onRunStateChange()?.(false);
    }
  }

  /** Stop an in-flight turn: ask the server to stop the work, and stop reading it here (the
   *  backend cannot tell a deliberate stop from a phone locking its screen). */
  cancel() {
    this.#turn?.cancel();
    this.#turn = null;
    this.active = false;
  }

  /** Run `start` in session `id`, which the chat has open. */
  async run(id: string, start: TurnStart) {
    const { alerts } = this.host;
    if (!alerts.error?.cutOff) alerts.error = null;
    this.active = true;
    this.host.onTurnStateChange()?.(true);
    // Before anything reaches the server: the host may be holding an edit on a timer, and
    // the run is about to snapshot and rewrite the very document that edit belongs to.
    const beforeTurn = this.host.beforeTurn();
    if (beforeTurn) {
      try {
        await beforeTurn();
      } catch {
        /* the host reports its own save failures; a turn is still worth starting */
      }
    }
    if (this.host.disposed()) return;
    // Whether the turn ever began. A message can queue behind the session's running turn and
    // then never start — the wait runs out, or it is stopped — and the composer cleared the
    // draft the moment it was sent. Without this the user's words would simply vanish.
    // A retry never emits user_prompt (that is the point), so "began" is not meaningful for it.
    let began = start.kind === 'retry';
    const watch = (event: TurnEvent) => {
      if (turnBegins(event)) began = true;
      this.host.event(id, event);
    };

    if (start.kind === 'autopilot') {
      this.runActive = true;
      this.host.onRunStateChange()?.(true);
    }
    const started = startTurnFor(id, start, watch, this.host.isSupport ? SUPPORT_BASE : undefined);
    this.#turn = started;
    this.host.followBottom();
    try {
      await started.done;
      if (!began && start.kind === 'message' && id === this.host.activeId()) {
        // The turn never ran, so the message was never recorded. Give it back rather than
        // losing it, and say why — the composer is empty and nothing else would explain it.
        this.host.giveBack(start);
        alerts.fail(this.host.text().messageNotSent);
      }
    } catch (err) {
      await this.#failed(id, start, err);
    }
  }

  async #failed(id: string, start: TurnStart, err: unknown) {
    const { alerts } = this.host;
    if (err instanceof StreamInterrupted) {
      // The stream broke, not the turn: it runs on the server under its own bounds and
      // stores everything it does. Marking the message as failed would tell the user
      // their CV edits were lost when they were not — so we re-read the session and show
      // whatever the agent has managed so far. A support session cannot be re-read that
      // way, so the rest of its answer is lost: say so rather than leave a silent cut. The
      // open answer is closed as an ordinary one (not errored — the server turn did not
      // fail), so it stops showing as streaming.
      if (this.host.isSupport && id === this.host.activeId()) {
        this.host.reduce({ type: 'result', stop_reason: 'end_turn' });
        alerts.cutOff(this.host.text().supportInterrupted);
      } else await this.host.reload(id);
      this.host.ended();
      return;
    }
    if (err instanceof TurnRefused) {
      // Neither remedy below ever started the turn, so the message was never recorded —
      // give it back to the composer rather than losing it while the candidate decides
      // what to do next.
      this.host.giveBack(start);
      if (err.canExtend) {
        // The session can be continued right now by spending another of the day's
        // tailoring sessions.
        alerts.refusal = err;
      } else {
        // A spent daily allowance: Retry would only repeat the same refusal, and there is
        // no assistant frame to mark errored (the turn never started), so this skips the
        // generic error result entirely rather than fabricating a dead-end Retry button.
        alerts.limit(err.message);
      }
      this.host.ended();
      return;
    }
    alerts.fail(err instanceof Error ? err.message : this.host.text().sendFailed);
    this.host.reduce({ type: 'result', stop_reason: 'error', is_error: true });
    this.host.ended();
  }
}
