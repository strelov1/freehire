// The history feed's pure logic: which preview nodes a revision lit up, how a run's edits
// fold into one entry, and how each hand is named. Kept out of the components so it can be
// unit-tested — the components themselves have no runner.
import type { RevisionView } from '$lib/generated/contracts';

/** An item that remembers where it sat before anything was filtered out. */
export type Positioned<T> = { index: number; item: T };

/**
 * keepIndex filters a list while carrying each survivor's ORIGINAL position.
 *
 * The preview hides empty entries and blank bullets, and a plain `.filter` renumbers what is
 * left. Revisions address the stored document, so one blank bullet between two filled ones
 * would send a highlight for `experience[0].bullets[2]` to the wrong line — silently, since
 * a wrong-but-valid index looks exactly like a right one.
 */
export function keepIndex<T>(items: readonly T[], keep: (item: T) => boolean): Positioned<T>[] {
  const out: Positioned<T>[] = [];
  items.forEach((item, index) => {
    if (keep(item)) out.push({ index, item });
  });
  return out;
}

/**
 * makeHighlighter answers, for a node's address, whether the selected revision touched it.
 *
 * A node is lit when the revision changed it or changed something it sits inside: replacing a
 * whole experience entry lights every line of that entry, because "what changed" is the
 * question being asked. The containing node of a changed field is NOT lit — underlining a
 * whole entry because one bullet in it moved says less than saying nothing.
 */
export function makeHighlighter(paths: readonly string[]): (nodePath: string) => boolean {
  if (paths.length === 0) return () => false;
  return (nodePath: string) =>
    paths.some((p) => {
      if (nodePath === p) return true;
      if (!nodePath.startsWith(p)) return false;
      // `experience[1]` must not match `experience[10]`: only a field or an index may follow.
      const next = nodePath[p.length];
      return next === '.' || next === '[';
    });
}

/** One entry in the rendered feed: a lone change, or the run several changes belonged to. */
export type RevisionGroup =
  | { kind: 'single'; revision: RevisionView }
  | { kind: 'batch'; batchId: string; revisions: RevisionView[]; undoable: boolean };

/**
 * groupByBatch folds the edits of one agent turn into a single entry, so the feed reads the
 * way the candidate experienced it — one run, not eleven edits — while still letting them
 * open it and undo any one of them.
 *
 * Only consecutive edits of the same run fold together: two runs are two entries even when
 * nothing was done between them.
 */
export function groupByBatch(revisions: readonly RevisionView[]): RevisionGroup[] {
  const groups: RevisionGroup[] = [];
  for (const revision of revisions) {
    const batchId = revision.batch_id;
    const last = groups.at(-1);
    if (batchId && last?.kind === 'batch' && last.batchId === batchId) {
      last.revisions.push(revision);
      last.undoable ||= !revision.reverted;
      continue;
    }
    if (batchId) {
      groups.push({ kind: 'batch', batchId, revisions: [revision], undoable: !revision.reverted });
      continue;
    }
    groups.push({ kind: 'single', revision });
  }
  return groups;
}

/** Names a hand the way the candidate would refer to it. */
export function actorLabel(actor: string): string {
  switch (actor) {
    case 'candidate':
      return 'You';
    case 'agent':
      return 'Assistant';
    case 'system':
      return 'freehire';
    default:
      return actor;
  }
}
