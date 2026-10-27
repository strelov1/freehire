import type { Action } from 'svelte/action';

/**
 * Sizes a full-bleed page to fill the viewport from wherever it starts down, so whatever sits
 * at its foot (a chat composer) is always on screen.
 *
 * Measured, not assumed: what stands above the page is the header AND whichever self-gating
 * bands the root layout shows (the email-verification prompt, whose height changes when its
 * code form opens), and a fixed `100dvh - header` sized the page past the fold by exactly
 * their height. Neither the body nor the page itself can tell when a band changes — html and
 * body are pinned at 100% — so the two boxes observed are the root layout's `min-h-svh`
 * column and its <main>: a band that grows pushes the column past the viewport, and one that
 * shrinks or goes away hands the freed height to <main> (it is `flex-1`, and the column
 * cannot shrink below the viewport). A page that then refits moves neither back out of step,
 * so the measurement settles in one step.
 *
 * The element keeps a header-only height (`100dvh - 3.5rem`) for the server render; the
 * inline height this writes takes over once it hydrates.
 */
export const fillViewport: Action<HTMLElement> = (node) => {
  const main = node.closest('main');
  const measure = () => {
    const top = Math.max(0, node.getBoundingClientRect().top + window.scrollY);
    node.style.height = `calc(100dvh - ${top}px)`;
  };
  measure();
  if (!main) return;
  const observer = new ResizeObserver(measure);
  observer.observe(main);
  if (main.parentElement) observer.observe(main.parentElement);
  return { destroy: () => observer.disconnect() };
};
