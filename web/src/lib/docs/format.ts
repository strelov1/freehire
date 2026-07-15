// Shared presentation helpers for the API docs: the per-method colour scales and
// the inline-code renderer, kept in one place so a new HTTP method or a tweak to
// the code voice is a single edit rather than one per component.

/** Text-only method tint (nav rows, endpoint index). */
export const METHOD_TEXT: Record<string, string> = {
  GET: 'text-emerald-600 dark:text-emerald-400',
  POST: 'text-sky-600 dark:text-sky-400',
  PUT: 'text-violet-600 dark:text-violet-400',
  PATCH: 'text-amber-600 dark:text-amber-400',
  DELETE: 'text-rose-600 dark:text-rose-400',
};

/** Filled method chip (soft fill + ring + text) for the endpoint signature pill. */
export const METHOD_CHIP: Record<string, string> = {
  GET: 'bg-emerald-500/10 text-emerald-700 ring-emerald-500/25 dark:text-emerald-400',
  POST: 'bg-sky-500/10 text-sky-700 ring-sky-500/25 dark:text-sky-400',
  PUT: 'bg-violet-500/10 text-violet-700 ring-violet-500/25 dark:text-violet-400',
  PATCH: 'bg-amber-500/10 text-amber-700 ring-amber-500/25 dark:text-amber-400',
  DELETE: 'bg-rose-500/10 text-rose-700 ring-rose-500/25 dark:text-rose-400',
};

/** Render the backtick `code` spans in an authored api-spec string as inline
 *  <code>. Inputs are constants (no user data), so escaping entities then
 *  swapping backticks is a safe, dependency-free way to keep the code voice. */
export function inlineCode(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(
      /`([^`]+)`/g,
      '<code class="rounded bg-secondary px-1 py-0.5 font-mono text-[12px] text-foreground">$1</code>',
    );
}
