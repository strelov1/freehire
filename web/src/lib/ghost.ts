import type { Ghost } from './generated/contracts';
import { timeAgo } from './utils';

/** A rendered ghost badge: a tone, a hedged chip label, the `fired/total` scale, and
 *  the hover justification.
 *
 *  The wording is hedged at every level by design. The system observes facts about a
 *  posting — how long it has been up, whether the employer's own board carries it,
 *  whether people who applied were answered — and never an employer's intent. "Open
 *  240 days, found only on an aggregator" is a claim about observables; "ghost job"
 *  is a claim about a state of mind we cannot see. So the word `ghost` stays in the
 *  code and never reaches the interface. */
export interface GhostBadge {
  tone: 'warn' | 'muted';
  label: string;
  scale: string;
  tooltip: string;
}

/** One row of the checklist: a criterion, whether it fired, and the facts behind it
 *  (or an explicit "no data" when it did not). */
export interface GhostChecklistRow {
  code: string;
  label: string;
  detail: string;
  fired: boolean;
}

/** The criteria in the order the classifier reports them: structural first, then
 *  outcome. Mirrors internal/ghost's constants — a code here with no counterpart
 *  there renders a row that can never fire.
 *
 *  Exported because the /features/ghost-jobs landing explains them from this same
 *  array, and a test fails if a criterion joins the vocabulary without being
 *  explained there. A marketing page a test keeps honest. */
export const CRITERIA: { code: string; label: string; tier: 'structural' | 'outcome' }[] = [
  { code: 'evergreen_posting', label: 'Posting behaves as evergreen', tier: 'structural' },
  { code: 'ats_absent', label: "Not on the company's own careers board", tier: 'structural' },
  { code: 'silent_applications', label: 'Applications here went unanswered', tier: 'outcome' },
  { code: 'user_reports', label: 'People reported no response', tier: 'outcome' },
];

const LABELS: Record<string, { tone: 'warn' | 'muted'; label: string }> = {
  possible: { tone: 'muted', label: 'Possibly inactive' },
  likely: { tone: 'warn', label: 'Likely inactive' },
};

/** ghostBadge maps the served signal to a chip, or null when there is nothing to
 *  show. An unrecognised level yields null rather than an empty chip: a badge beside
 *  a job that says nothing is worse than no badge. */
export function ghostBadge(ghost?: Ghost | null): GhostBadge | null {
  if (!ghost) return null;
  const shape = LABELS[ghost.level];
  if (!shape) return null;

  const fired = ghost.criteria.length;
  const tooltip = ghostChecklist(ghost)
    .filter((r) => r.fired)
    .map((r) => r.label)
    .join(' · ');
  return {
    tone: shape.tone,
    label: shape.label,
    scale: `${fired}/${ghost.criteria_total}`,
    tooltip,
  };
}

/** supersedesReality reports whether the ghost badge replaces the reality badge.
 *
 *  `evergreen_posting` IS the reality verdict, so rendering both shows one fact
 *  twice, the second time louder. Where ghost is silent the reality badge renders
 *  unchanged. */
export function supersedesReality(ghost?: Ghost | null): boolean {
  return ghostBadge(ghost) !== null;
}

/** ghostChecklist renders every criterion the classifier considers — including the
 *  ones with nothing behind them.
 *
 *  Showing the unfired rows is the point rather than clutter: "no data" beside the
 *  outcome criteria tells the reader WHY the level is not higher, instead of leaving
 *  them to guess how serious this is. */
export function ghostChecklist(ghost: Ghost): GhostChecklistRow[] {
  const fired = new Set(ghost.criteria);
  return CRITERIA.map(({ code, label }) => ({
    code,
    label,
    fired: fired.has(code),
    detail: detailFor(code, ghost, fired.has(code)),
  }));
}

function detailFor(code: string, ghost: Ghost, fired: boolean): string {
  if (!fired) return 'No data';
  switch (code) {
    case 'ats_absent': {
      const ago = ghost.ats_checked_at ? timeAgo(ghost.ats_checked_at) : '';
      return ago ? `Checked ${ago}` : 'Checked against the company board';
    }
    case 'silent_applications':
    case 'user_reports':
      // The contributor count is served only above the anonymity gate, and the UI
      // must not invent one below it: a count of one identifies that applicant to
      // the employer. Absent means absent.
      return ghost.contributors ? `From ${ghost.contributors} people` : 'Reported';
    default:
      return 'Yes';
  }
}
