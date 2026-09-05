import type { Action } from 'svelte/action';

/**
 * Next focus index for a horizontal tablist, or `null` when `key` is not a
 * navigation key. Left/Right wrap around; Home/End jump to the ends. Pure so it can
 * be unit-tested in Node (the DOM glue below is verified via svelte-check + visually).
 */
export function nextTabIndex(current: number, key: string, count: number): number | null {
  if (count <= 0) return null;
  switch (key) {
    case 'ArrowRight':
      return (current + 1) % count;
    case 'ArrowLeft':
      return (current - 1 + count) % count;
    case 'Home':
      return 0;
    case 'End':
      return count - 1;
    default:
      return null;
  }
}

/**
 * WAI-ARIA tabs keyboard behaviour on a `role="tablist"` container: roving tabindex
 * (only the selected tab is in the Tab sequence) plus Left/Right/Home/End to move
 * focus between tabs. Activation is **manual** — arrows only move focus, and the
 * native Enter/Space (button) or Enter (link) activates — so link-based tabs never
 * navigate on an arrow press.
 *
 * The relationships (`role="tabpanel"`, `id`, `aria-controls`/`aria-labelledby`) are
 * wired declaratively in each tablist's markup; this action owns only the behaviour.
 *
 * Pass the active tab identifier as the parameter so the roving tabindex re-syncs
 * whenever the selection (or route, for link tabs) changes.
 */
export const tablist: Action<HTMLElement, unknown> = (node) => {
  const tabs = () => Array.from(node.querySelectorAll<HTMLElement>('[role="tab"]'));

  // Only the selected tab (or the first, if none) stays tabbable.
  const syncTabindex = () => {
    const items = tabs();
    const selected = items.findIndex((el) => el.getAttribute('aria-selected') === 'true');
    const active = selected === -1 ? 0 : selected;
    items.forEach((el, i) => (el.tabIndex = i === active ? 0 : -1));
  };

  const onKeydown = (e: KeyboardEvent) => {
    const items = tabs();
    const current = items.indexOf(document.activeElement as HTMLElement);
    if (current === -1) return;
    const next = nextTabIndex(current, e.key, items.length);
    if (next === null) return;
    e.preventDefault();
    items[next]?.focus();
  };

  node.addEventListener('keydown', onKeydown);
  syncTabindex();

  return {
    update: syncTabindex,
    destroy: () => node.removeEventListener('keydown', onKeydown),
  };
};
