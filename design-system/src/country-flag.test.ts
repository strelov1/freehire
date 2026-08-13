import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import CountryFlag from './country-flag.svelte';
import { must } from './test-utils';

describe('CountryFlag', () => {
  it('renders the flag-icons glyph for a valid two-letter code', () => {
    const { container } = render(CountryFlag, { code: 'de', label: 'Germany' });

    const el = must(container.querySelector('span'));
    expect(el.className).toContain('fi-de');
    expect(el.getAttribute('role')).toBe('img');
    expect(el.getAttribute('aria-label')).toBe('Germany');
    expect(el.getAttribute('title')).toBe('Germany');
  });

  it('accepts a code in any case and with surrounding whitespace', () => {
    const { container } = render(CountryFlag, { code: ' DE ', label: 'Germany' });

    expect(must(container.querySelector('span')).className).toContain('fi-de');
  });

  it('falls back to the plain code when it has no flag in the sheet', () => {
    const { container, getByText } = render(CountryFlag, { code: 'remote', label: 'Remote' });

    expect(container.querySelector('.fi')).toBeNull();
    const el = getByText('REMOTE');
    expect(el.getAttribute('title')).toBe('Remote');
    // The fallback carries no image semantics — it is plain labelled text, not a graphic.
    expect(el.getAttribute('role')).toBeNull();
  });
});
