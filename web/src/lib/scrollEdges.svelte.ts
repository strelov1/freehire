// Whether a horizontally scrolling row has content past either edge — what decides which
// edge fades. Measured rather than assumed: an unconditional fade would paint a phantom
// edge on a row that already fits. The design system's TabStrip keeps its own copy of the
// same rule.

export class ScrollEdges {
  atStart = $state(true);
  atEnd = $state(true);

  /** `{@attach edges.attach}` on the scrolling element. */
  attach = (el: HTMLElement) => {
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
      observer.disconnect();
      el.removeEventListener('scroll', measure);
    };
  };
}
