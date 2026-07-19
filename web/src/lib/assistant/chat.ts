// Pure presentation logic for the assistant chat: folds the streamed
// `TurnEvent`s into a message list, kept out of the component so the
// accumulation is unit-testable (vitest) without a DOM — mirroring how
// `matchAnalysis.ts` isolates `reduceMatchEvent`.

import type { TurnEvent } from './wire';
import type { ToolCall } from './tool-formatters';

/** One rendered message. Assistant messages accumulate `text` (the reply),
 *  `thinking` (secondary, never mixed into the reply), and `tools` (each tool
 *  call's `{name, input}`, so the renderer can show details like the bash
 *  command or the file read) while `streaming`; `errored` is set when the turn
 *  ends with an error. User messages carry only `text`. */
export interface ChatMessage {
  role: 'user' | 'assistant';
  text: string;
  thinking: string;
  tools: ToolCall[];
  streaming: boolean;
  errored: boolean;
}

export interface ChatState {
  messages: ChatMessage[];
}

export function initChat(): ChatState {
  return { messages: [] };
}

/** Fold one `TurnEvent` into the chat state, returning a new state (never
 *  mutates the input). Unmodeled/unknown event kinds are ignored (no throw). */
export function reduceTurnEvent(prev: ChatState, event: TurnEvent): ChatState {
  switch (event.type) {
    case 'user_prompt':
      return { messages: [...prev.messages, userMessage(event.text)] };
    case 'assistant_text':
      return upsertAssistant(prev, (m) => ({ ...m, text: m.text + event.text }));
    case 'assistant_thought':
      return upsertAssistant(prev, (m) => ({ ...m, thinking: m.thinking + event.text }));
    case 'tool_use':
      return upsertAssistant(prev, (m) => ({
        ...m,
        tools: [...m.tools, { name: event.name, input: event.input }],
      }));
    case 'result':
      return closeAssistant(prev, event.is_error);
    default:
      // system, note, usage, raw, and anything not in the union — ignored.
      return prev;
  }
}

function userMessage(text: string): ChatMessage {
  return { role: 'user', text, thinking: '', tools: [], streaming: false, errored: false };
}

function newAssistant(): ChatMessage {
  return { role: 'assistant', text: '', thinking: '', tools: [], streaming: true, errored: false };
}

/** Apply `fn` to the open (streaming) assistant message, or start a new one if
 *  the last message isn't an open assistant turn (e.g. after a user prompt or a
 *  closed turn). */
function upsertAssistant(prev: ChatState, fn: (m: ChatMessage) => ChatMessage): ChatState {
  const last = prev.messages[prev.messages.length - 1];
  if (last && last.role === 'assistant' && last.streaming) {
    return { messages: [...prev.messages.slice(0, -1), fn(last)] };
  }
  return { messages: [...prev.messages, fn(newAssistant())] };
}

/** Close the open assistant turn. A result with no open turn is ignored (never
 *  fabricates an empty message). */
function closeAssistant(prev: ChatState, errored: boolean): ChatState {
  const last = prev.messages[prev.messages.length - 1];
  if (!last || last.role !== 'assistant' || !last.streaming) return prev;
  const closed: ChatMessage = { ...last, streaming: false, errored };
  return { messages: [...prev.messages.slice(0, -1), closed] };
}
