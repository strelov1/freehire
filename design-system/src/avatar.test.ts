import { render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import Avatar from './avatar.svelte';
import { must } from './test-utils';

function circle(name?: string) {
  const { container } = render(Avatar, { name });
  return must(container.querySelector('div'));
}

/** jsdom resolves the authored `hsl()` down to `rgb(r, g, b)`. */
function rgb(value: string): [number, number, number] {
  const parts = value.match(/\d+/g)?.map(Number) ?? [];
  return [parts[0] ?? 0, parts[1] ?? 0, parts[2] ?? 0];
}

/** WCAG 2.1 relative luminance. */
function luminance([r, g, b]: [number, number, number]) {
  const [rl, gl, bl] = [r, g, b].map((c) => {
    const v = c / 255;
    return v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  }) as [number, number, number];
  return 0.2126 * rl + 0.7152 * gl + 0.0722 * bl;
}

function contrast(el: HTMLElement) {
  const [ink, fill] = [luminance(rgb(el.style.color)), luminance(rgb(el.style.backgroundColor))];
  return (Math.max(ink, fill) + 0.05) / (Math.min(ink, fill) + 0.05);
}

describe('Avatar', () => {
  // The regression: the fill was hardcoded but the ink came from --foreground,
  // which inverts by theme — the dark theme put near-white initials on this
  // near-white fill.
  it('paints the ink itself instead of inheriting a themed one', () => {
    const el = circle('Ada Lovelace');

    expect(el.style.color).not.toBe('');
    expect(el.style.backgroundColor).not.toBe('');
    expect(el.className).not.toContain('text-foreground');
  });

  // Both halves of the pair are fixed, so the ratio holds in either theme —
  // which is the whole point. 26 names is enough to land on every part of the
  // hue circle; the tightest is around yellow, at ~6:1.
  it('keeps the initials readable whatever hue the name lands on', () => {
    const names = Array.from({ length: 26 }, (_, i) => `${String.fromCharCode(65 + i)} Person`);

    for (const name of names) {
      expect(contrast(circle(name)), `contrast for ${name}`).toBeGreaterThanOrEqual(4.5);
    }
  });

  it('keeps the fill stable for a given name and varies it between names', () => {
    const ada = circle('Ada Lovelace').style.backgroundColor;
    const again = circle('Ada Lovelace').style.backgroundColor;
    const grace = circle('Grace Hopper').style.backgroundColor;

    expect(again).toBe(ada);
    expect(grace).not.toBe(ada);
  });

  it('falls to grey without a name', () => {
    const [r, g, b] = rgb(circle().style.backgroundColor);

    expect(r).toBe(g);
    expect(g).toBe(b);
  });

  it('is named by the person, not spelled out as initials', () => {
    const el = circle('Ada Lovelace');

    expect(el.getAttribute('role')).toBe('img');
    expect(el.getAttribute('aria-label')).toBe('Ada Lovelace');
    expect(el.textContent?.trim()).toBe('AL');
  });

  it('stays out of the accessibility tree when it carries no name', () => {
    const el = circle();

    expect(el.getAttribute('aria-hidden')).toBe('true');
    expect(el.getAttribute('role')).toBeNull();
    expect(el.textContent?.trim()).toBe('?');
  });

  it('takes at most two initials', () => {
    expect(circle('Ada Byron King Lovelace').textContent?.trim()).toBe('AB');
    expect(circle('Prince').textContent?.trim()).toBe('P');
  });

  // A pasted résumé/profile field is a common source of stray whitespace; the split
  // must not turn it into a leading empty "initial".
  it('ignores leading, trailing, and repeated whitespace when deriving initials', () => {
    expect(circle(' Ada Lovelace').textContent?.trim()).toBe('AL');
    expect(circle('Ada Lovelace ').textContent?.trim()).toBe('AL');
    expect(circle('Ada  Lovelace').textContent?.trim()).toBe('AL');
  });

  it('leaves a photo undescribed when there is no name to describe it with', () => {
    const { container } = render(Avatar, { src: 'https://example.test/a.png' });

    expect(must(container.querySelector('img')).getAttribute('alt')).toBe('');
  });

  // Promoted from CompanyLogo (see openspec/changes/promote-shared-components-to-design-system).
  describe('shape', () => {
    it('is a circle by default', () => {
      const el = circle('Ada Lovelace');
      expect(el.className).toContain('rounded-full');
    });

    it('becomes a rounded square when asked', () => {
      const { container } = render(Avatar, { name: 'Ada Lovelace', shape: 'square' });
      const el = must(container.querySelector('div'));

      expect(el.className).toContain('rounded');
      expect(el.className).not.toContain('rounded-full');
    });

    it('applies the same shape to a photo', () => {
      const { container } = render(Avatar, { src: 'https://example.test/a.png', shape: 'square' });
      const img = must(container.querySelector('img'));

      expect(img.className).toContain('rounded');
      expect(img.className).not.toContain('rounded-full');
    });

    // A square (entity-mark) photo is always shown beside the name as visible text at every
    // real call site, so it stays decorative; a circular (person) photo may be the sole
    // identification and keeps its accessible name.
    it('leaves a square photo decorative, since the name sits beside it as text', () => {
      const { container } = render(Avatar, { name: 'Acme Corp', src: 'https://example.test/a.png', shape: 'square' });

      expect(must(container.querySelector('img')).getAttribute('alt')).toBe('');
    });

    it('keeps a circular photo named', () => {
      const { container } = render(Avatar, { name: 'Ada Lovelace', src: 'https://example.test/a.png' });

      expect(must(container.querySelector('img')).getAttribute('alt')).toBe('Ada Lovelace');
    });
  });

  describe('a broken photo', () => {
    it('falls back to the initials render once the image fails to load', async () => {
      const { container } = render(Avatar, { name: 'Ada Lovelace', src: 'https://example.test/404.png' });

      const img = must(container.querySelector('img'));
      img.dispatchEvent(new Event('error'));
      await Promise.resolve();

      expect(container.querySelector('img')).toBeNull();
      const fallback = must(container.querySelector('div'));
      expect(fallback.textContent?.trim()).toBe('AL');
    });

    it('resets the failure when the src changes, giving the new photo a fresh attempt', async () => {
      const { container, rerender } = render(Avatar, {
        name: 'Ada Lovelace',
        src: 'https://example.test/404.png',
      });
      must(container.querySelector('img')).dispatchEvent(new Event('error'));
      await Promise.resolve();
      expect(container.querySelector('img')).toBeNull();

      await rerender({ name: 'Ada Lovelace', src: 'https://example.test/ok.png' });

      expect(container.querySelector('img')).not.toBeNull();
    });
  });

  describe('fallbackIcon', () => {
    const icon = createRawSnippet(() => ({ render: () => '<svg data-testid="icon"></svg>' }));

    it('is used instead of "?" when there is no name and no photo', () => {
      const { container } = render(Avatar, { fallbackIcon: icon });

      expect(container.querySelector('[data-testid="icon"]')).not.toBeNull();
      expect(container.textContent?.trim()).toBe('');
    });

    it('is not shown once a name is given, even without a photo', () => {
      const { container, getByText } = render(Avatar, { name: 'Ada Lovelace', fallbackIcon: icon });

      expect(container.querySelector('[data-testid="icon"]')).toBeNull();
      expect(getByText('AL')).toBeTruthy();
    });

    it('leaves the default "?" alone when no fallbackIcon is given', () => {
      expect(circle().textContent?.trim()).toBe('?');
    });
  });
});
