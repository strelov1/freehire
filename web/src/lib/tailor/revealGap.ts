import { announce } from '#lib/announce.svelte.js';
import type { Placeholder } from './readiness';
import { revealSelection } from './revealSelection';

/**
 * Underlines-and-goes-to one slot: scrolls the preview to its mark, announces `message`,
 * and puts the cursor on the slot in the Editor with the bracketed text selected, so typing
 * replaces it. The field is found by the same address the check produced
 * (`experience[1].bullets[0]`), which the Editor tags each input with.
 */
export function revealGap(
  gap: Placeholder,
  preview: HTMLElement | null | undefined,
  leftPanel: HTMLElement | null | undefined,
  message: string,
): void {
  const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  const behavior = reduce ? 'auto' : 'smooth';
  // The hidden measurement layer renders the same marks, so skip anything inside it.
  const mark = Array.from(preview?.querySelectorAll('.cv-lit') ?? []).find((el) => !el.closest('[aria-hidden="true"]'));
  mark?.scrollIntoView({ block: 'center', behavior });
  const field = leftPanel?.querySelector<HTMLInputElement | HTMLTextAreaElement>(
    `[data-path="${CSS.escape(gap.path)}"]`,
  );
  announce(message);
  // A run locks the form (a disabled field takes no focus); the underline still shows where.
  if (!field || field.disabled) return;
  field.scrollIntoView({ block: 'center', behavior });
  field.focus({ preventScroll: true });
  const at = field.value.indexOf(gap.token);
  if (at < 0) return;
  field.setSelectionRange(at, at + gap.token.length);
  revealSelection(field, at);
}
