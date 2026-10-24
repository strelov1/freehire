import { prefersReducedMotion } from 'svelte/motion';

/** The `behavior` for a scripted scroll: smooth, unless the visitor asked for reduced motion. */
export function scrollBehavior(): ScrollBehavior {
  return prefersReducedMotion.current ? 'auto' : 'smooth';
}
