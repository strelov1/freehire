import { describe, expect, it } from 'vitest';
import { DETECTORS, RADII } from './check-token-coverage.mjs';

const find = (kind, line) => DETECTORS[kind](line);

describe('colour literal', () => {
  const kind = 'colour literal';

  it('finds a hex, short or long', () => {
    expect(find(kind, 'color: #fff')).toEqual(['#fff']);
    expect(find(kind, 'color: #ff8800')).toEqual(['#ff8800']);
  });

  it('finds every colour function, whatever the space', () => {
    expect(find(kind, 'rgb(0 0 0)')).toEqual(['rgb(']);
    expect(find(kind, 'hsl(200 50% 50%)')).toEqual(['hsl(']);
    expect(find(kind, 'oklch(0.5 0.1 200)')).toEqual(['oklch(']);
  });

  it('leaves ordinary source alone', () => {
    expect(find(kind, 'const size = compute(value)')).toEqual([]);
  });
});

describe('Tailwind arbitrary value', () => {
  const kind = 'Tailwind arbitrary value';

  it('finds a bracketed value', () => {
    expect(find(kind, 'class="p-[7px]"')).toEqual(['-[7px]']);
    expect(find(kind, 'class="w-[calc(100%-2rem)]"')).toEqual(['-[calc(100%-2rem)]']);
  });

  // An arbitrary *variant* is a selector, not a value — no token could replace
  // it, and the `:` after the bracket is the whole discriminator.
  it('leaves an arbitrary variant alone', () => {
    expect(find(kind, 'class="[&_tr]:border-b"')).toEqual([]);
  });

  // The leading hyphen is what keeps TypeScript out of it.
  it('leaves TypeScript indexing alone', () => {
    expect(find(kind, 'const cls = sizes[size];')).toEqual([]);
    expect(find(kind, 'let nodes: HTMLButtonElement[] = [];')).toEqual([]);
  });
});

describe('raw palette utility', () => {
  const kind = 'raw palette utility';

  // The detector web actually needs. A hue off Tailwind's built-in palette is
  // neither a literal nor an arbitrary value — it is a well-formed utility the
  // other two detectors cannot see, and it is the majority of what web/src has.
  it('finds a hue at a shade', () => {
    expect(find(kind, 'class="text-amber-600"')).toEqual(['text-amber-600']);
    expect(find(kind, 'class="bg-emerald-500"')).toEqual(['bg-emerald-500']);
    expect(find(kind, 'class="border-red-300"')).toEqual(['border-red-300']);
  });

  it('finds one behind a variant prefix', () => {
    expect(find(kind, 'class="hover:bg-blue-700 dark:text-rose-400"')).toEqual([
      'bg-blue-700',
      'text-rose-400',
    ]);
  });

  it('leaves a semantic token utility alone', () => {
    expect(find(kind, 'class="bg-card text-muted-foreground border-border"')).toEqual([]);
  });

  // Only colour scales. Every other Tailwind scale is spacing, size or order,
  // and none of them is a colour the theme owns.
  it('leaves the non-colour scales alone', () => {
    expect(find(kind, 'class="p-4 gap-2 z-50 grid-cols-3"')).toEqual([]);
  });
});

describe('radii', () => {
  // Two radii, deliberately not the same rule. The package has fifteen files and
  // is clean, so a violation there is always a mistake; web has 216 and cannot
  // reach zero in one change, so it is a number instead.
  it('judges the package by literals and arbitrary values only', () => {
    expect(RADII.package).toEqual(['colour literal', 'Tailwind arbitrary value']);
  });

  it('adds the palette detector for web', () => {
    expect(RADII.web).toContain('raw palette utility');
  });
});
