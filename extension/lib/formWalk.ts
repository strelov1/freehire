import { browser } from 'wxt/browser';
import type { LabelFill, RuntimeMessage } from './protocol';
import { applyStep, nextStep, skipStep, startWalk, stopWalk, type Walk } from './walk';

/** What a walk tells the panel driving it, and asks of it. */
export interface WalkHooks {
  /** The question being filled now, or null once the walk is over. */
  filling(label: string | null): void;
  /** Whether Stop has been pressed. */
  stopped(): boolean;
  /** The walk wrote into the page's form. */
  wrote(): void;
  /** A question was answered; `frame` and `form` narrow it when the caller knows them. */
  answered(label: string, frame?: number, form?: number): void;
}

/** How long each filled question stays in view before the walk moves on. It is
 *  there for the eye: without it the walk is a flicker, which is the batch fill
 *  this replaced. */
const WALK_STEP_MS = 300;

function pause(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Works through `fills` one question at a time, each one revealed as its value
 * lands, ticking the plan off as it goes. Returns what it managed — the caller
 * owns the closing sentence, because the two callers reach the walk from
 * different places (its own plan, or the agent's report).
 */
export async function walkFills(fills: LabelFill[], hooks: WalkHooks): Promise<Walk<LabelFill>> {
  let walk = startWalk(fills);
  try {
    // `nextStep` is the continuation test: it ends the walk both when the list
    // runs out and when Stop has been pressed.
    for (let fill = nextStep(walk); fill !== null; fill = nextStep(walk)) {
      hooks.filling(fill.label);
      const applied = (await browser.runtime.sendMessage({
        kind: 'FILL_BY_LABEL',
        fills: [fill],
        reveal: true,
      } satisfies RuntimeMessage)) as RuntimeMessage | undefined;
      const outcome = applied?.kind === 'FILL_OUTCOMES' ? applied.outcomes[0] : undefined;
      if (outcome?.status === 'filled') {
        walk = applyStep(walk, fill);
        hooks.wrote();
        // Ticked off from what was just written, not from a fresh read: the
        // page's own change notice is debounced, so re-reading here would leave
        // the counter a step behind the value on screen.
        hooks.answered(fill.label, fill.frame, fill.form);
      } else {
        walk = skipStep(walk, fill);
      }
      if (hooks.stopped()) return stopWalk(walk);
      await pause(WALK_STEP_MS);
    }
    return walk;
  } finally {
    hooks.filling(null);
  }
}

/** Plays a list of labels the agent says it filled back over the page, so the
 *  user sees what changed. Nothing is written — the agent already did that. */
export async function walkReported(labels: string[], hooks: WalkHooks): Promise<void> {
  try {
    for (const label of labels) {
      if (hooks.stopped()) return;
      hooks.filling(label);
      await browser.runtime.sendMessage({
        kind: 'REVEAL_FIELD',
        request: { label },
      } satisfies RuntimeMessage);
      hooks.answered(label);
      await pause(WALK_STEP_MS);
    }
  } finally {
    // Whatever ended the walk — the list, Stop, or a throw — no question is
    // being filled once it is over, and a stuck spinner would say otherwise.
    hooks.filling(null);
  }
}
