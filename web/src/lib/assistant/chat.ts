// Pure presentation logic for the assistant chat: folds the streamed
// `TurnEvent`s into a message list, kept out of the component so the
// accumulation is unit-testable (vitest) without a DOM — mirroring how
// `matchAnalysis.ts` isolates `reduceMatchEvent`.

import type { TurnEvent } from './wire';
import type { ToolCall } from './tool-formatters';

/** One rendered message. Assistant messages accumulate `text` (the reply),
 *  `thinking` (secondary, never mixed into the reply), and `tools` (each tool
 *  call with its arguments and, once it comes back, its result) while
 *  `streaming`; `errored` is set when the turn ends with an error. User messages
 *  carry only `text`. */
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
  /** The turn is waiting for another one to finish in this session. Its own frames have not
   *  started, so without this the view would show a silent stream and nothing else. */
  queued: boolean;
}

export function initChat(): ChatState {
  return { messages: [], queued: false };
}

/** Fold one `TurnEvent` into the chat state, returning a new state (never
 *  mutates the input). Unmodeled/unknown event kinds are ignored (no throw). */
export function reduceTurnEvent(prev: ChatState, event: TurnEvent): ChatState {
  switch (event.type) {
    case 'queued':
      return { ...prev, queued: true };
    case 'user_prompt':
      // The turn is under way: whatever wait preceded it is over.
      return { ...prev, messages: [...prev.messages, userMessage(event.text)], queued: false };
    case 'assistant_text':
      return upsertAssistant(prev, (m) => ({ ...m, text: m.text + event.text }));
    case 'assistant_thought':
      return upsertAssistant(prev, (m) => ({ ...m, thinking: m.thinking + event.text }));
    case 'tool_use':
      return upsertAssistant(prev, (m) => ({
        ...m,
        tools: [...m.tools, { name: event.name, input: event.input }],
      }));
    case 'tool_result':
      return upsertAssistant(prev, (m) => ({ ...m, tools: attachResult(m.tools, event) }));
    case 'result':
      // Clears the wait as well as closing the turn: a wait can end WITHOUT the turn ever
      // starting (it timed out, or was stopped), and a banner still claiming to be queued
      // would then mask the next turn's real progress.
      return { ...closeAssistant(prev, event.is_error ?? false), queued: false };
    default:
      // usage, and anything not in the union — ignored.
      return prev;
  }
}

/** Attach a result to the call it answers: the FIRST still-unanswered call of
 *  that name. The backend runs a round's calls in order and emits their results
 *  in the same order, so first-unanswered is the right match — taking the most
 *  recent one instead would hand the second search's result to the first search.
 *  Keying by name rather than by call id keeps the reducer free of id
 *  bookkeeping; a result for a call we never saw is dropped rather than
 *  fabricating a card. */
function attachResult(
  tools: ToolCall[],
  event: { name: string; result?: string; is_error?: boolean },
): ToolCall[] {
  for (let i = 0; i < tools.length; i++) {
    const tool = tools[i];
    if (!tool || tool.name !== event.name || tool.result !== undefined) continue;
    const answered: ToolCall = {
      name: tool.name,
      input: tool.input,
      result: event.result ?? '',
      isError: event.is_error === true,
    };
    return [...tools.slice(0, i), answered, ...tools.slice(i + 1)];
  }
  return tools;
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
    return { ...prev, messages: [...prev.messages.slice(0, -1), fn(last)] };
  }
  return { ...prev, messages: [...prev.messages, fn(newAssistant())] };
}

/** Close the open assistant turn. A result with no open turn is ignored (never
 *  fabricates an empty message). */
function closeAssistant(prev: ChatState, errored: boolean): ChatState {
  const last = prev.messages[prev.messages.length - 1];
  if (!last || last.role !== 'assistant' || !last.streaming) return prev;
  const closed: ChatMessage = { ...last, streaming: false, errored };
  return { ...prev, messages: [...prev.messages.slice(0, -1), closed] };
}
