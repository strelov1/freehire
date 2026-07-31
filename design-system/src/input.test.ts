import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import Input from './input.svelte';

const field = (props: Record<string, unknown> = {}) =>
  render(Input, props).container.querySelector('input') as HTMLInputElement;

describe('Input', () => {
  // Input has no props of its own beyond `value` and `class` — everything that
  // makes it accessible arrives through the spread. FormField hands its control
  // { id, describedBy, required, invalid } and the call site wires them onto
  // these attributes, so the passthrough IS the contract between the two
  // primitives, not an incidental convenience.
  it('passes the accessibility attributes a FormField supplies', () => {
    const input = field({
      id: 'email',
      'aria-describedby': 'email-error',
      'aria-invalid': 'true',
      required: true,
    });

    expect(input.id).toBe('email');
    expect(input.getAttribute('aria-describedby')).toBe('email-error');
    expect(input.getAttribute('aria-invalid')).toBe('true');
    expect(input.required).toBe(true);
  });

  it('passes the type through', () => {
    expect(field({ type: 'search' }).type).toBe('search');
    expect(field({ type: 'email' }).type).toBe('email');
  });

  it('passes placeholder and disabled through', () => {
    const input = field({ placeholder: 'Search jobs', disabled: true });

    expect(input.placeholder).toBe('Search jobs');
    expect(input.disabled).toBe(true);
  });

  it('renders the value it is given', () => {
    expect(field({ value: 'golang' }).value).toBe('golang');
  });

  // No width is baked in on purpose — call sites pass w-full or flex-1 — so the
  // class prop is not decoration here, it is how the primitive is sized at all.
  it('lets a caller override a base class', () => {
    const input = field({ class: 'h-20 rounded-none' });

    expect(input.className).toContain('h-20');
    expect(input.className).not.toContain('h-9');
    expect(input.className).not.toContain('rounded-lg');
  });
});
