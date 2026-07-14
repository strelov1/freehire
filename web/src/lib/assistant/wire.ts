// TypeScript mirror of the roy control protocol (`ClientCommand`/`ServerEvent`)
// and the `TurnEvent` enum, ported from roy-web (`workspace/src/lib/wire.ts`).
// The agent backend (`freehire-agent`) speaks this protocol verbatim over its
// `/ws` relay: the client sends `ClientCommand` JSON and receives `ServerEvent`
// JSON, one JSON value per WebSocket message. The backend is the source of
// truth for this shape; this port is thin and read-only.

export type Harness = 'claude' | 'gemini' | 'opencode' | 'codex' | 'pi';

/** Shape returned by `listed`. `session, harness, model, cwd` come from the
 *  daemon; `project_id`/`tags` may be spliced in server-side. */
export interface SessionInfo {
  session: string;
  harness: string;
  cwd: string;
  model?: string;
  project_id?: string;
  tags?: Record<string, string>;
}

export type Seq = number;

// ---- TurnEvent (tag: "type") ---------------------------------------------

export type StopReason =
  | 'end_turn'
  | 'max_tokens'
  | 'max_turn_requests'
  | 'refusal'
  | 'cancelled'
  | 'error'
  | (string & {});

export type TurnEvent =
  | { type: 'system'; subtype: string }
  | { type: 'user_prompt'; text: string }
  | { type: 'note'; text: string; source_session: string | null }
  | { type: 'assistant_text'; text: string }
  | { type: 'assistant_thought'; text: string }
  | { type: 'tool_use'; name: string; input: unknown }
  | {
      type: 'usage';
      input_tokens: number | null;
      output_tokens: number | null;
      cost_usd: number | null;
    }
  | {
      type: 'result';
      cost_usd: number | null;
      stop_reason: StopReason;
      is_error: boolean;
    }
  | { type: 'raw'; value: unknown };

export interface JournalEntry {
  seq: Seq;
  /// Wall-clock millis since epoch, stamped by the daemon when the entry hit
  /// the journal. UIs render this as the send/receive time of the message.
  ts_ms: number;
  event: TurnEvent;
}

// ---- ClientCommand (tag: "op") -------------------------------------------

export type ClientCommand =
  | { op: 'attach'; session: string; from_seq?: Seq }
  | { op: 'acquire_input'; session: string }
  | { op: 'send'; session: string; text: string }
  | { op: 'cancel_turn'; session: string }
  | { op: 'set_model'; session: string; model: string }
  | { op: 'release_input'; session: string }
  | { op: 'detach'; session: string }
  | { op: 'close'; session: string }
  | { op: 'delete_archive'; session: string }
  | { op: 'list' }
  | { op: 'list_archived' }
  | { op: 'list_harnesses' }
  | { op: 'resume'; session: string }
  | {
      op: 'read_journal';
      session: string;
      from_seq?: Seq;
      max_entries?: number;
    };

// ---- ServerEvent (tag: "kind") -------------------------------------------

export type ErrorCode =
  | 'bad_request'
  | 'spawn_failed'
  | 'no_session'
  | 'attach_failed'
  | 'archive_read_failed'
  | 'no_lease'
  | 'send_failed'
  | 'close_failed'
  | 'list_archived_failed'
  | 'resume_failed'
  | 'read_journal_failed'
  | 'delete_failed'
  | 'cancel_failed'
  | 'set_model_failed'
  | (string & {});

export type ServerEvent =
  | { kind: 'resuming'; session: string }
  | {
      kind: 'attached';
      session: string;
      seq_at_attach: Seq;
      harness: string;
      model?: string;
    }
  | { kind: 'frame'; session: string; entry: JournalEntry }
  | { kind: 'input_acquired'; session: string; acquired: boolean }
  | { kind: 'input_released'; session: string }
  | { kind: 'detached'; session: string }
  | { kind: 'model_changed'; session: string; model: string }
  | { kind: 'closed'; session: string }
  | { kind: 'deleted'; session: string }
  | { kind: 'listed'; sessions: SessionInfo[] }
  | { kind: 'listed_archived'; sessions: SessionInfo[] }
  | { kind: 'resumed'; session: string; resume_cursor?: string }
  | {
      kind: 'journal_read';
      session: string;
      entries: JournalEntry[];
      next_seq: Seq;
      has_more: boolean;
    }
  | { kind: 'error'; session?: string; code: ErrorCode; message: string };

export type ServerEventKind = ServerEvent['kind'];
