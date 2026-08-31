// The tailoring ATS delta as the workspace shows it: signed numbers, and — when readability
// fell — a warning that names the category responsible. Kept out of the component so the
// wording and the sign handling are unit-tested rather than eyeballed in a browser.
import type { CvAtsDelta } from '$lib/cv';
import type { LineItem } from '$lib/generated/contracts';

interface AtsDeltaRow {
  id: string;
  label: string;
  base: number;
  tailored: number;
  change: number;
  /** The change as shown: '+8', '−15', or '0'. */
  text: string;
  /** The checks behind the tailored side's score, for the row's disclosure. Always an
   *  array: the wire omits it when empty, and a row the panel cannot iterate would take the
   *  whole panel down over a category that simply had nothing to say. */
  items: LineItem[];
}

export interface AtsDeltaView {
  overall: { base: number; tailored: number; change: number; text: string };
  regressed: boolean;
  /** Set only on a regression: what fell, by how much, and where. */
  warning?: string;
  rows: AtsDeltaRow[];
}

/** signed renders a change with a real minus sign (U+2212), not a hyphen: these numbers sit
 *  next to scores, and a hyphen there reads as a range. */
function signed(change: number): string {
  if (change > 0) return `+${change}`;
  if (change < 0) return `−${Math.abs(change)}`;
  return '0';
}

/**
 * viewAtsDelta turns the endpoint's response into what the panel renders, or null when there
 * is nothing to show — an unavailable delta is an absence, not an error state, so the caller
 * has one falsy case to handle instead of two.
 */
export function viewAtsDelta(resp: CvAtsDelta | null | undefined): AtsDeltaView | null {
  const delta = resp?.available ? resp.delta : undefined;
  if (!delta) return null;

  const rows: AtsDeltaRow[] = (delta.categories ?? []).map((c) => ({
    id: c.id,
    label: c.label,
    base: c.base,
    tailored: c.tailored,
    change: c.change,
    text: signed(c.change),
    items: c.items ?? [],
  }));

  const view: AtsDeltaView = {
    overall: {
      base: delta.base,
      tailored: delta.tailored,
      change: delta.change,
      // A bare '0' beside a score reads as a score. Say it in words instead.
      text: delta.change === 0 ? 'no change' : signed(delta.change),
    },
    regressed: delta.regressed,
    rows,
  };

  if (delta.regressed) {
    const points = Math.abs(delta.change);
    // The server names the worst category by id; an id this client does not know (an older SPA
    // against a newer API) drops the clause rather than rendering 'undefined'.
    const worst = rows.find((r) => r.id === delta.worst_category);
    const where = worst ? `, most of it in ${worst.label}` : '';
    view.warning = `Tailoring lowered ATS readability by ${points} ${points === 1 ? 'point' : 'points'}${where}.`;
  }
  return view;
}

/** Which direction a change points, for the caller to map to its own styling. A regression is
 *  its own tone rather than plain 'down': the overall score can fall while some category rose. */
export type AtsDeltaTone = 'down' | 'up' | 'flat';

export function toneOf(change: number, regressed = false): AtsDeltaTone {
  if (regressed || change < 0) return 'down';
  if (change > 0) return 'up';
  return 'flat';
}
