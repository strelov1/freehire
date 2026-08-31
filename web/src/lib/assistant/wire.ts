// The assistant's wire contract, mirroring `internal/assistant`'s Event and the
// stored transcript. The agent runs inside the freehire backend, so a turn is one
// HTTP request whose body is a stream of named SSE events — there is no control
// protocol, no session attach and no input lease any more.
//
// The backend is the source of truth for these shapes; this port is thin and
// read-only.

/** Why a turn ended. Every turn ends with exactly one `result`. */
type StopReason = 'end_turn' | 'max_steps' | 'cancelled' | 'error' | (string & {});

/** One streamed frame of a turn. */
export type TurnEvent =
  // The turn has not started: it is waiting for the one in flight in this session. Sent
  // ahead of the turn's own frames so a wait does not read as a stalled connection.
  | { type: 'queued' }
  | { type: 'user_prompt'; text: string }
  | { type: 'assistant_text'; text: string }
  | { type: 'assistant_thought'; text: string }
  | { type: 'tool_use'; name: string; input?: unknown }
  | { type: 'tool_result'; name: string; result?: string; is_error?: boolean }
  | { type: 'usage'; input_tokens?: number; output_tokens?: number }
  | { type: 'result'; stop_reason?: StopReason; is_error?: boolean };

/** A conversation in the session rail. */
export interface SessionSummary {
  id: string;
  preset: 'chat' | 'tailor' | (string & {});
  label: string;
  cv_id?: number;
  job_id?: number;
}

/** One stored transcript message. `content` is role-specific: a prompt, one model
 *  turn (text plus the tool calls it asked for), or one tool's result. */
export interface StoredMessage {
  seq: number;
  role: 'user' | 'assistant' | 'tool';
  content: unknown;
}

interface UserContent {
  text?: string;
}
interface AssistantContent {
  text?: string;
  tool_calls?: { id: string; name: string; arguments: string }[];
}
interface ToolContent {
  tool_call_id?: string;
  name?: string;
  result?: string;
}

/**
 * Replay a stored transcript as the same events a live turn emits, so history and
 * a running turn fold through one reducer. A `result` is synthesised before each
 * new prompt and at the end, because the transcript records what was said, not
 * when a turn closed — without them every replayed assistant message would still
 * look like it was streaming.
 */
export function eventsFromTranscript(messages: StoredMessage[]): TurnEvent[] {
  const out: TurnEvent[] = [];
  let open = false;

  const close = () => {
    if (open) {
      out.push({ type: 'result', stop_reason: 'end_turn' });
      open = false;
    }
  };

  for (const message of messages) {
    if (message.role === 'user') {
      close();
      out.push({ type: 'user_prompt', text: (message.content as UserContent)?.text ?? '' });
      continue;
    }
    if (message.role === 'assistant') {
      const content = (message.content ?? {}) as AssistantContent;
      if (content.text) {
        out.push({ type: 'assistant_text', text: content.text });
        open = true;
      }
      for (const call of content.tool_calls ?? []) {
        out.push({ type: 'tool_use', name: call.name, input: parseArguments(call.arguments) });
        open = true;
      }
      continue;
    }
    if (message.role === 'tool') {
      const content = (message.content ?? {}) as ToolContent;
      out.push({
        type: 'tool_result',
        name: content.name ?? '',
        result: content.result,
        is_error: isErrorResult(content.result),
      });
      open = true;
    }
  }
  close();
  return out;
}

/** Tool arguments are stored as the model wrote them, so a malformed call must
 *  still render — fall back to the raw string rather than dropping the card. */
function parseArguments(raw: string | undefined): unknown {
  if (!raw) return {};
  try {
    return JSON.parse(raw) as unknown;
  } catch {
    return raw;
  }
}

/** A failed tool result is the error envelope the registry renders. The stored
 *  transcript keeps no separate flag, so the envelope is what identifies it. */
function isErrorResult(result: string | undefined): boolean {
  if (!result) return false;
  try {
    const parsed = JSON.parse(result) as Record<string, unknown>;
    return typeof parsed?.error === 'string';
  } catch {
    return false;
  }
}
