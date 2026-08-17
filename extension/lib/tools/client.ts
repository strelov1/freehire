/**
 * The extension's end of the browser-tool wire: one WebSocket to hire's relay,
 * over which a harness drives this browser. The socket lives in the side panel —
 * the only context that stays alive while the user is working (the MV3 service
 * worker is killed when idle, so it must not hold a durable connection).
 *
 * Auth: browsers cannot set headers on `new WebSocket`, so the session JWT rides
 * the subprotocol slot next to a literal marker, which is all the relay echoes
 * back. (The HTTP side of the panel sends the same token as a Bearer header.)
 */

import { HIRE_ORIGIN } from '../auth';
import { executeTool, type PageBridge } from './executor';
import { parseToolCall } from './wire';

/** The subprotocol marker hire's relay accepts; the JWT rides beside it. */
export const WS_SUBPROTOCOL_MARKER = 'freehire-jwt';

/** The relay endpoint, joined as the extension end of this user's channel. */
export function toolsWsUrl(): string {
  return `${HIRE_ORIGIN.replace(/^http/, 'ws')}/api/v1/tools/ws?role=extension`;
}

/**
 * Runs one inbound wire frame and returns the frame to send back, or null when
 * the input is not a tool call we can correlate a reply to.
 */
export async function respondTo(data: unknown, page: PageBridge): Promise<string | null> {
  const call = parseToolCall(data);
  if (!call) return null;
  return JSON.stringify(await executeTool(call, page));
}

export type ChannelStatus = 'idle' | 'connecting' | 'open' | 'closed' | 'unreachable';

// A rejected handshake (expired/revoked token, relay down, ...) reaches the client
// as the same opaque close event a network blip would — the WebSocket API never
// exposes the HTTP status a failed upgrade got, so there is no way to tell "stop
// retrying this token" apart from "try again in a moment" from here. Retrying
// forever is still right (this is what makes a restarting relay self-heal), but
// retrying SILENTLY forever is not: a stale token would otherwise fail every 30s
// with nothing on screen to explain why autofill and the browse tool never work.
// After this many consecutive failures, 'unreachable' fires once so the panel can
// tell the user, without breaking off the retry loop that recovers a transient
// outage on its own.
const GIVE_UP_NOTICE_AFTER_ATTEMPTS = 5;

/**
 * Keeps the extension attached to the relay, answering tool calls for as long as
 * the panel is open. Reconnects on an unexpected drop with a widening backoff, so
 * a restarting relay does not become a hot loop.
 */
export class ToolChannel {
  private ws: WebSocket | null = null;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private attempt = 0;
  private stopped = true;
  private notifiedUnreachable = false;

  constructor(
    private readonly page: PageBridge,
    private readonly onStatus: (s: ChannelStatus) => void = () => {},
  ) {}

  /** Attaches, and keeps re-attaching, until `stop()`. */
  start(token: string): void {
    this.stopped = false;
    this.attempt = 0;
    this.notifiedUnreachable = false;
    this.open(token);
  }

  stop(): void {
    this.stopped = true;
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
    this.ws?.close();
    this.ws = null;
    this.onStatus('closed');
  }

  private open(token: string): void {
    this.onStatus('connecting');
    const ws = new WebSocket(toolsWsUrl(), [WS_SUBPROTOCOL_MARKER, token]);
    this.ws = ws;

    ws.onopen = () => {
      this.attempt = 0;
      this.notifiedUnreachable = false;
      this.onStatus('open');
    };
    ws.onmessage = async (ev) => {
      const reply = await respondTo(ev.data, this.page);
      if (reply && ws.readyState === WebSocket.OPEN) ws.send(reply);
    };
    ws.onclose = () => {
      this.onStatus('closed');
      this.retry(token);
    };
    // A socket that errors also closes; leave the retry to onclose so a failure
    // is not scheduled twice.
    ws.onerror = () => {};
  }

  private retry(token: string): void {
    if (this.stopped) return;
    this.attempt++;
    if (this.attempt >= GIVE_UP_NOTICE_AFTER_ATTEMPTS && !this.notifiedUnreachable) {
      this.notifiedUnreachable = true;
      this.onStatus('unreachable');
    }
    const delay = Math.min(30_000, 1_000 * 2 ** (this.attempt - 1));
    this.timer = setTimeout(() => this.open(token), delay);
  }
}
