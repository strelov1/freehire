import { render } from '@testing-library/svelte';
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

  it('leaves a photo undescribed when there is no name to describe it with', () => {
    const { container } = render(Avatar, { src: 'https://example.test/a.png' });

    expect(must(container.querySelector('img')).getAttribute('alt')).toBe('');
  });
});
