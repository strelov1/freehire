/**
 * The extension's half of the browser-tool wire: it turns a `ToolCall` into a
 * `ToolResult` by driving a `PageBridge`. Pure over the bridge — the chrome
 * plumbing that actually reaches the tab's frames lives in `page.ts`, so the
 * dispatch, the argument validation and the error contract are testable without
 * a browser.
 */

import type {
  LabelFill,
  FillOutcome,
  FillStatus,
  ComboboxStep,
  ComboboxReply,
  PageSnapshot,
} from '../protocol';
import type { FramedField, FramedUpload, ToolCall, ToolResult } from './wire';

/** What one read of the page reports: its questions, and its resume uploads. */
export interface FormObservation {
  fields: FramedField[];
  /**
   * The uploads on offer. Not fill targets — they travel because they are what
   * marks a form as an application rather than a job-alert signup, and nothing
   * downstream can see them otherwise.
   */
  uploads: FramedUpload[];
}

/** Whatever can read and write the page the user is looking at. */
export interface PageBridge {
  /** What the tab is showing, read live: url, title, headline and visible text. */
  readPage(): Promise<PageSnapshot>;
  /** Every frame's observation, folded into one, each entry tagged with its frame. */
  readForm(): Promise<FormObservation>;
  /** Applies the fills across the page's frames, one outcome per requested fill. */
  fillSimple(fills: LabelFill[]): Promise<FillOutcome[]>;
  /** Runs one step against the widget, wherever in the page's frames it lives. */
  combobox(step: ComboboxStep): Promise<ComboboxReply>;
}

/** The wire's combobox tools, and the step each one runs. */
const COMBOBOX_TOOLS: Record<string, ComboboxStep['action']> = {
  'combobox.open': 'open',
  'combobox.options': 'options',
  'combobox.select': 'select',
  'combobox.verify': 'verify',
};

/**
 * Runs one tool call. Never throws: an unknown tool, bad arguments, or a failing
 * bridge all come back as an error result tagged with the call id, because the
 * harness on the other end is waiting on that id and silence would hang it.
 */
export async function executeTool(call: ToolCall, page: PageBridge): Promise<ToolResult> {
  try {
    switch (call.tool) {
      // The posting's prose, not the application form's fields — this is what the
      // assistant reads when the user refers to the page in front of them.
      case 'read_page':
        return { id: call.id, result: await page.readPage() };
      case 'read_form':
        return { id: call.id, result: await page.readForm() };
      case 'fill_simple': {
        const fills = readFills(call.args);
        return { id: call.id, result: { outcomes: await page.fillSimple(fills) } };
      }
      default: {
        const action = COMBOBOX_TOOLS[call.tool];
        if (!action) return { id: call.id, error: `unknown tool: ${call.tool}` };
        return { id: call.id, result: await page.combobox(readComboboxStep(action, call.args)) };
      }
    }
  } catch (err) {
    return { id: call.id, error: err instanceof Error ? err.message : String(err) };
  }
}

/** Validates `fill_simple`'s arguments, throwing the message the caller sees. */
function readFills(args: Record<string, unknown> | undefined): LabelFill[] {
  const fills = args?.fills;
  if (!Array.isArray(fills)) throw new Error('fill_simple requires args.fills: [{label, value}]');
  return fills.map((f) => {
    const { label, value } = (f ?? {}) as Record<string, unknown>;
    if (typeof label !== 'string' || !label) throw new Error('every fill needs a non-empty label');
    return { label, value: typeof value === 'string' ? value : String(value ?? '') };
  });
}

/**
 * Validates a combobox call's arguments. `select` and `verify` need the option in
 * play — a blank would either commit nothing or confirm nothing, both of which
 * are better refused than reported as a result.
 */
function readComboboxStep(action: ComboboxStep['action'], args: Record<string, unknown> | undefined): ComboboxStep {
  const { label, value } = args ?? {};
  if (typeof label !== 'string' || !label) throw new Error(`combobox.${action} requires args.label`);
  if ((action === 'select' || action === 'verify') && (typeof value !== 'string' || !value)) {
    throw new Error(`combobox.${action} requires args.value: the option in play`);
  }
  return { action, label, value: typeof value === 'string' ? value : '' };
}

/**
 * Folds the frames' answers into one. A widget lives in exactly one frame, so
 * every other frame reports `not_found` and the one informative answer wins.
 */
export function mergeComboboxReplies(replies: ComboboxReply[]): ComboboxReply {
  return replies.find((r) => r.status !== 'not_found') ?? { status: 'not_found' };
}

// A frame that does not contain the field reports `not_found`, so across frames
// the informative answer must win over the many negatives. Ordered least to most
// informative; ties keep the first frame that reported it.
const STATUS_RANK: Record<FillStatus, number> = {
  not_found: 0,
  no_option: 1,
  deferred_combobox: 2,
  filled: 3,
};

/** Folds each frame's outcomes into one answer per requested label. */
export function mergeFrameOutcomes(perFrame: FillOutcome[][]): FillOutcome[] {
  const best = new Map<string, FillOutcome>();
  for (const outcomes of perFrame) {
    for (const outcome of outcomes) {
      const seen = best.get(outcome.label);
      if (!seen || STATUS_RANK[outcome.status] > STATUS_RANK[seen.status]) {
        best.set(outcome.label, outcome);
      }
    }
  }
  return [...best.values()];
}
