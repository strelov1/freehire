// Whether a horizontally scrolling row has content past either edge — what decides which
// edge fades. Measured rather than assumed: an unconditional fade would paint a phantom
// edge on a row that already fits. The design system's TabStrip keeps its own copy of the
// same rule.

export class ScrollEdges {
  atStart = $state(true);
  atEnd = $state(true);

  #el: HTMLElement | null = null;

  /** `{@attach edges.attach}` on the scrolling element. */
  attach = (el: HTMLElement) => {
    this.#el = el;
    const measure = () => {
      this.atStart = el.scrollLeft <= 1;
      // 1px of slack — fractional widths leave scrollLeft just shy of its true maximum.
      this.atEnd = el.scrollLeft >= el.scrollWidth - el.clientWidth - 1;
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    for (const child of el.children) observer.observe(child);
    el.addEventListener('scroll', measure, { passive: true });
    return () => {
      if (this.#el === el) this.#el = null;
      observer.disconnect();
      el.removeEventListener('scroll', measure);
    };
  };

  /**
   * Scroll the attached row so `child` is fully in view, with a little room past it. The
   * row is scrolled itself rather than through scrollIntoView, which would also drag the
   * page (or the panel) the row sits in.
   */
  reveal(child: HTMLElement) {
    const el = this.#el;
    if (!el) return;
    const target = child.getBoundingClientRect();
    const view = el.getBoundingClientRect();
    if (target.right > view.right) el.scrollLeft += target.right - view.right + 24;
    else if (target.left < view.left) el.scrollLeft -= view.left - target.left + 24;
  }
}
