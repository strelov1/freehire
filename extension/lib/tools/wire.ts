/**
 * The browser-tool wire: the harness-agnostic frames a "brain" and this extension
 * exchange through hire's relay. Raw JSON, request/response, correlated by `id` —
 * an MCP wrapper is a later concern.
 *
 * Two brains drive it today, both inside hire: the agentic autofill, and the
 * assistant's `read_current_page` when a conversation runs the `browse` preset.
 *
 * This file is the contract; keep it in step with hire's relay
 * (`internal/browsertools`) which forwards these frames verbatim.
 */

import type { FramedField, FramedUpload } from '../protocol';

export type { FramedField, FramedUpload };

/** A tool invocation travelling harness → extension. */
export interface ToolCall {
  id: string;
  tool: string;
  args?: Record<string, unknown>;
}

/**
 * The answer travelling extension → harness. Exactly one of `result`/`error` is
 * set: a failed tool comes back as an error result rather than as silence, so a
 * call never hangs waiting for a reply that will not arrive.
 */
export interface ToolResult {
  id: string;
  result?: unknown;
  error?: string;
}

/**
 * Parses an inbound wire frame, returning null when it is not a well-formed tool
 * call. A malformed frame cannot be answered (there is no id to correlate to), so
 * the caller drops it rather than guessing.
 */
export function parseToolCall(data: unknown): ToolCall | null {
  if (typeof data !== 'string') return null;
  let parsed: unknown;
  try {
    parsed = JSON.parse(data);
  } catch {
    return null;
  }
  if (!parsed || typeof parsed !== 'object') return null;
  const { id, tool, args } = parsed as Record<string, unknown>;
  if (typeof id !== 'string' || !id || typeof tool !== 'string' || !tool) return null;
  return {
    id,
    tool,
    args: args && typeof args === 'object' ? (args as Record<string, unknown>) : {},
  };
}
