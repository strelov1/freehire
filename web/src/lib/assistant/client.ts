import {
  type ClientCommand,
  type JournalEntry,
  type ServerEvent,
  type ServerEventKind,
} from './wire';

// The browser half of the roy control protocol, ported from roy-web
// (`workspace/src/lib/client.ts`). One `RoyClient` owns one WebSocket to the
// agent relay: `call` sends a command and awaits its matching reply, `fire` is
// fire-and-forget (`send`), and frame events flow to `subscribeFrames`. Unlike
// roy-web this exports no module singleton — the chat page owns a per-page
// instance and closes it on teardown (SvelteKit route lifecycle, not HMR).

export type ConnectionStatus = 'idle' | 'connecting' | 'open' | 'closed' | 'error';

/** An unsolicited backend error (not a reply to a pending `call`). */
export type ServerError = { code: string; message: string; session?: string };

type Pending = {
  expected: ServerEventKind;
  resolve: (ev: ServerEvent) => void;
  reject: (err: Error) => void;
};

export class RoyClient {
  private ws: WebSocket | null = null;
  private queue: Pending[] = [];
  private frameSubs = new Map<string, Set<(entry: JournalEntry) => void>>();
  private statusSubs = new Set<(s: ConnectionStatus) => void>();
  private errorSubs = new Set<(err: ServerError) => void>();
  private _status: ConnectionStatus = 'idle';

  get status(): ConnectionStatus {
    return this._status;
  }

  /**
   * Connect to the agent relay's WebSocket. Resolves when the socket reaches
   * OPEN, rejects if it errors, closes before opening, or fails to upgrade
   * within `timeoutMs` (default 10s).
   *
   * Auth is UNIFIED with freehire: the relay authenticates from the freehire
   * `hire_token` cookie the browser sends automatically on the same-origin
   * upgrade (shared JWT secret), so no token is passed here.
   *
   * Without the timeout a half-open TCP socket (backend crashed mid-handshake,
   * network stall) leaves `onopen`/`onerror` silent and the returned Promise
   * pends forever, hanging the awaiting caller.
   */
  connect(url: string, timeoutMs = 10_000): Promise<void> {
    if (this.ws) this.close();
    this.setStatus('connecting');
    return new Promise((resolve, reject) => {
      const ws = new WebSocket(url);
      this.ws = ws;
      const timer = setTimeout(() => {
        this.setStatus('error');
        try {
          ws.close();
        } catch {
          // already closed
        }
        reject(new Error(`timed out connecting to ${url} after ${timeoutMs}ms`));
      }, timeoutMs);
      ws.onopen = () => {
        clearTimeout(timer);
        this.setStatus('open');
        resolve();
      };
      ws.onerror = () => {
        clearTimeout(timer);
        this.setStatus('error');
        if (ws.readyState !== WebSocket.OPEN) {
          reject(new Error(`failed to connect to ${url}`));
        }
        this.flushQueueWithError('websocket error');
      };
      ws.onclose = () => {
        clearTimeout(timer);
        this.setStatus('closed');
        this.flushQueueWithError('websocket closed');
      };
      ws.onmessage = (msg) => this.handleMessage(msg.data);
    });
  }

  close() {
    this.ws?.close();
    this.ws = null;
  }

  /**
   * Send a command and await the matching server reply. Frame events are not
   * counted as replies — they flow through `subscribeFrames` instead.
   *
   * Rejects with the Error event's message if the backend answered with an
   * Error of any code.
   */
  call<K extends ServerEventKind>(
    cmd: ClientCommand,
    expected: K,
  ): Promise<Extract<ServerEvent, { kind: K }>> {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return Promise.reject(new Error('not connected'));
    }
    return new Promise((resolve, reject) => {
      this.queue.push({
        expected,
        resolve: (ev) => resolve(ev as Extract<ServerEvent, { kind: K }>),
        reject,
      });
      this.ws!.send(JSON.stringify(cmd));
    });
  }

  /**
   * Fire-and-forget command. `send` is the canonical case: the backend emits no
   * ack on success — the observable effect is the stream of `frame` events that
   * follows, terminated by a `result`. Only an `error` event can come back, and
   * that resolves whatever command is at the head of the queue (so don't
   * interleave `fire` with pending `call`s).
   */
  fire(cmd: ClientCommand) {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error('not connected');
    }
    this.ws.send(JSON.stringify(cmd));
  }

  /**
   * Subscribe to live frame events for a session. Returns an unsubscribe
   * function.
   */
  subscribeFrames(session: string, cb: (entry: JournalEntry) => void): () => void {
    let set = this.frameSubs.get(session);
    if (!set) {
      set = new Set();
      this.frameSubs.set(session, set);
    }
    set.add(cb);
    return () => {
      const s = this.frameSubs.get(session);
      s?.delete(cb);
      if (s && s.size === 0) this.frameSubs.delete(session);
    };
  }

  onStatus(cb: (s: ConnectionStatus) => void): () => void {
    this.statusSubs.add(cb);
    cb(this._status);
    return () => this.statusSubs.delete(cb);
  }

  /**
   * Subscribe to backend `error` events that arrive UNsolicited — i.e. not as
   * the reply to a pending `call`. A fire-and-forget `send` produces no pending
   * entry, so if a turn fails with `{kind:'error'}` instead of a terminal
   * `result` it lands here; without a subscriber the turn would hang forever.
   */
  onError(cb: (err: ServerError) => void): () => void {
    this.errorSubs.add(cb);
    return () => this.errorSubs.delete(cb);
  }

  private setStatus(s: ConnectionStatus) {
    this._status = s;
    for (const cb of this.statusSubs) cb(s);
  }

  private handleMessage(data: unknown) {
    if (typeof data !== 'string') return;
    let ev: ServerEvent;
    try {
      ev = JSON.parse(data) as ServerEvent;
    } catch (e) {
      console.error('assistant: invalid JSON from backend', e, data);
      return;
    }

    if (ev.kind === 'frame') {
      this.dispatchFrame(ev.session, ev.entry);
      return;
    }

    // Progress ack, not the awaited reply — returning here (instead of
    // dequeuing) keeps FIFO matching aligned for the real reply behind it.
    if (ev.kind === 'resuming') {
      return;
    }

    // Every other event resolves the head of the pending queue. The backend
    // processes commands serially per connection, so FIFO matching is sound.
    const pending = this.queue.shift();
    if (!pending) {
      // A `send` is fire-and-forget (no pending entry). If a turn fails with an
      // `error` instead of a terminal `result`, it lands here — route it to
      // `onError` subscribers so the turn can be ended instead of hanging.
      if (ev.kind === 'error') {
        for (const cb of this.errorSubs) cb({ code: ev.code, message: ev.message, session: ev.session });
      } else {
        console.warn('assistant: unsolicited event', ev);
      }
      return;
    }
    if (ev.kind === 'error') {
      pending.reject(new Error(`${ev.code}: ${ev.message}`));
      return;
    }
    if (ev.kind !== pending.expected) {
      pending.reject(new Error(`expected ${pending.expected}, got ${ev.kind}`));
      return;
    }
    pending.resolve(ev);
  }

  private dispatchFrame(session: string, entry: JournalEntry) {
    const set = this.frameSubs.get(session);
    if (!set) return;
    for (const cb of set) cb(entry);
  }

  private flushQueueWithError(reason: string) {
    const q = this.queue;
    this.queue = [];
    for (const p of q) p.reject(new Error(reason));
  }
}
