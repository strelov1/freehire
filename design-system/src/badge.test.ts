import { render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import Badge, { type BadgeVariant } from './badge.svelte';

const slot = (html: string) => createRawSnippet(() => ({ render: () => html }));

function classesFor(props: { variant?: BadgeVariant; class?: string }) {
  const { container, unmount } = render(Badge, { ...props, children: slot('Remote') });
  const classes = (container.querySelector('span') as HTMLSpanElement).className;
  unmount();
  return classes;
}

describe('Badge', () => {
  const VARIANTS: BadgeVariant[] = ['secondary', 'outline', 'brand', 'missing'];

  it('gives every variant its own classes', () => {
    const seen = VARIANTS.map((variant) => classesFor({ variant }));

    expect(new Set(seen).size).toBe(VARIANTS.length);
  });

  it('falls back to secondary', () => {
    expect(classesFor({})).toBe(classesFor({ variant: 'secondary' }));
  });

  // `missing` is the variant that shipped without its destructive tint in phase
  // 4: the classes were right and Tailwind dropped them, because the package
  // resolves under node_modules and v4's automatic source detection ignores it.
  // That failure is invisible from here — a class name in an attribute is all a
  // unit test can see, and whether Tailwind emitted a rule for it is the
  // @source scan's business. This asserts the half that is testable; the other
  // half is CI's assertion over storybook-static, and the eye.
  it('names the destructive tint on the missing variant', () => {
    expect(classesFor({ variant: 'missing' })).toContain('text-destructive/90');
  });

  it('lets a caller override a base class', () => {
    const classes = classesFor({ class: 'rounded-full' });

    expect(classes).toContain('rounded-full');
    expect(classes).not.toContain('rounded-md');
  });
});
