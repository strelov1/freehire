import { bulletCapUserMessage } from '#lib/assistant/bulletCapAlert.js';
import type { TurnRefused } from '#lib/assistant/client.js';
import type { TurnEvent } from '#lib/assistant/wire.js';

/** What the chat's banners say: AssistantAlerts renders it, ChatGone renders `notFound`. */
export class ChatAlerts {
  // planLimitReached distinguishes a spent daily allowance (which offers a link to the
  // plan) from every other failure this component reports through the same banner — one
  // object so the two facts can never fall out of step with each other the way two
  // separately-reset variables did (a stale flag surviving past the error it described).
  error = $state<{ message: string; planLimitReached: boolean; cutOff?: boolean } | null>(null);
  // A ceiling refusal is not a dead end: the session can be continued by spending another
  // of the day's tailoring sessions. Held apart from `error` because it needs an action
  // beside it, and because a plain error message would tell the candidate to give up on
  // work they can carry on with right now.
  refusal = $state<TurnRefused | null>(null);
  // Distinct from `error`: the URL names a conversation that is not the caller's to
  // open — deleted, someone else's, or a tailoring chat that belongs to a CV. It is
  // a dead link, not a failure, so it gets an explanation and a way out.
  notFound = $state(false);
  // When cv_edit refuses an insert that would silently truncate a full role, show a
  // dismissible banner — the tool chip alone is easy to miss, and the candidate needs
  // to know their existing bullets were kept.
  bulletCap = $state<string | null>(null);

  /** Every ordinary failure goes through here. */
  fail(message: string) {
    this.error = { message, planLimitReached: false };
  }

  /** A spent daily allowance: the banner links to the plan. */
  limit(message: string) {
    this.error = { message, planLimitReached: true };
  }

  // A support answer cut short by a dropped stream. Unlike other errors it is not cleared
  // when the next turn starts: a message queued behind the dropped turn starts that turn the
  // moment this one ends, before the visitor could read it. Only the visitor's own next
  // action (sending, or starting or switching conversations) removes it.
  cutOff(message: string) {
    this.error = { message, planLimitReached: false, cutOff: true };
  }

  /** The bullet-cap notice follows the stream: a refused insert raises it, the next message clears it. */
  noteEvent(event: TurnEvent) {
    if (event.type === 'user_prompt') this.bulletCap = null;
    if (event.type === 'tool_result' && event.is_error) {
      const alert = bulletCapUserMessage(event.result);
      if (alert) this.bulletCap = alert;
    }
  }
}
