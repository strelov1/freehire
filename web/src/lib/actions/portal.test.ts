import { describe, expect, it } from 'vitest';
import { trapsOverlays } from './portal';

// A fake element tree: each node knows its parent and its style. Enough to exercise the
// rule without a DOM, the same split focusTrap uses.
function tree(...styles: Partial<CSSStyleDeclaration>[]) {
  const styleOf = new Map<Element, Partial<CSSStyleDeclaration>>();
  let parent: Element | null = null;
  let leaf: Element = null as unknown as Element;
  // Outermost first, so the last style given is the leaf's own parent.
  for (const style of styles) {
    const el = { parentElement: parent } as unknown as Element;
    styleOf.set(el, style);
    parent = el;
    leaf = el;
  }
  const node = { parentElement: leaf } as unknown as Element;
  return { node, styleOf: (el: Element) => styleOf.get(el) ?? {} };
}

describe('trapsOverlays', () => {
  // The one that actually shipped: the job list's filter sidebar is sticky, so a dialog
  // rendered inside it painted under the job cards that come later in the document.
  it('finds a sticky ancestor', () => {
    const { node, styleOf } = tree({ position: 'static' }, { position: 'sticky' });
    expect(trapsOverlays(styleOf, node)).toBe(true);
  });

  it('finds a transformed ancestor', () => {
    const { node, styleOf } = tree({ transform: 'translateZ(0)' });
    expect(trapsOverlays(styleOf, node)).toBe(true);
  });

  it('ignores an explicit none', () => {
    const { node, styleOf } = tree({ transform: 'none', filter: 'none', perspective: 'none' });
    expect(trapsOverlays(styleOf, node)).toBe(false);
  });

  it('is false when nothing between the node and the body opens a layer', () => {
    const { node, styleOf } = tree({ position: 'static' }, { position: 'relative' });
    expect(trapsOverlays(styleOf, node)).toBe(false);
  });
});
