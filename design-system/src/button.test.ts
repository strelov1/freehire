import { render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it } from 'vitest';
import Button, { type ButtonSize, type ButtonVariant } from './button.svelte';

const slot = (html: string) => createRawSnippet(() => ({ render: () => html }));

function classesFor(props: { variant?: ButtonVariant; size?: ButtonSize; class?: string }) {
  const { getByRole, unmount } = render(Button, { ...props, children: slot('Go') });
  const classes = getByRole('button').className;
  unmount();
  return classes;
}

describe('Button', () => {
  // The drift this is placed to catch: `destructive` was added to the variants
  // and never reached the hand-kept argTypes, and nothing was red. A variant
  // that stops resolving falls back to the default silently — same classes, no
  // error, a button that looks secondary where the call site asked for danger.
  const VARIANTS: ButtonVariant[] = ['primary', 'secondary', 'outline', 'ghost', 'destructive'];

  it('gives every variant its own classes', () => {
    const seen = VARIANTS.map((variant) => classesFor({ variant }));

    expect(new Set(seen).size).toBe(VARIANTS.length);
  });

  const SIZES: ButtonSize[] = ['sm', 'md', 'lg', 'icon'];

  it('gives every size its own classes', () => {
    const seen = SIZES.map((size) => classesFor({ size }));

    expect(new Set(seen).size).toBe(SIZES.length);
  });

  it('falls back to secondary at medium', () => {
    expect(classesFor({})).toBe(classesFor({ variant: 'secondary', size: 'md' }));
  });

  // A <button> inside a <form> submits it by default, which is almost never what
  // a call site means. The primitive says type="button" for them — and a call
  // site that does want a submit has to be able to say so, which only works
  // because the spread comes after the attribute.
  it('is a plain button by default', () => {
    const { getByRole } = render(Button, { children: slot('Go') });

    expect(getByRole('button').getAttribute('type')).toBe('button');
  });

  it('lets the caller ask for a submit button', () => {
    const { getByRole } = render(Button, { type: 'submit', children: slot('Go') });

    expect(getByRole('button').getAttribute('type')).toBe('submit');
  });

  it('passes native attributes through', () => {
    const { getByRole } = render(Button, {
      disabled: true,
      'aria-label': 'Dismiss',
      children: slot('Go'),
    });

    expect((getByRole('button') as HTMLButtonElement).disabled).toBe(true);
    expect(getByRole('button').getAttribute('aria-label')).toBe('Dismiss');
  });

  it('becomes an anchor when given an href', () => {
    const { getByRole, queryByRole } = render(Button, {
      href: '/jobs',
      children: slot('Go'),
    });

    expect(getByRole('link').getAttribute('href')).toBe('/jobs');
    expect(queryByRole('button')).toBeNull();
  });

  // Reverse-tabnabbing guard: a target="_blank" anchor must not keep a window.opener
  // handle back to this page, and a caller must not have to remember rel by hand.
  it('adds rel=noopener noreferrer by default when opened in a new tab', () => {
    // `target` doubles as a testing-library render option, so it — and every prop
    // alongside it — has to go under `props` explicitly here.
    const { getByRole } = render(Button, {
      props: {
        href: 'https://example.com',
        target: '_blank',
        children: slot('Go'),
      },
    });

    expect(getByRole('link').getAttribute('rel')).toBe('noopener noreferrer');
  });

  it('lets a caller override the default rel', () => {
    const { getByRole } = render(Button, {
      props: {
        href: 'https://example.com',
        target: '_blank',
        rel: 'noopener',
        children: slot('Go'),
      },
    });

    expect(getByRole('link').getAttribute('rel')).toBe('noopener');
  });

  it('leaves rel unset for a same-tab link', () => {
    const { getByRole } = render(Button, {
      href: '/jobs',
      children: slot('Go'),
    });

    expect(getByRole('link').getAttribute('rel')).toBeNull();
  });

  // Every call site that passes a class assumes it wins. It only does because cn
  // runs tailwind-merge — plain concatenation would leave both in the list and
  // let source order in the stylesheet decide.
  it('lets a caller override a base class', () => {
    const classes = classesFor({ class: 'rounded-full' });

    expect(classes).toContain('rounded-full');
    expect(classes).not.toContain('rounded-md');
  });

  it('lets a caller override a class the size set', () => {
    const classes = classesFor({ size: 'md', class: 'h-20' });

    expect(classes).toContain('h-20');
    expect(classes).not.toContain('h-9');
  });
});
