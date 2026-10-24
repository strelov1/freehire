import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import PillGroup from './PillGroup.svelte';

// A filter pill's state is its fill and a strikethrough — neither reaches a screen
// reader. Each pill must say it programmatically (see pillPressed in pill.ts), and its
// count must not run into the label in the accessible name. (jsdom's name computation
// trims the hidden ", " to ","; a browser keeps the space, hence the optional \s.)

const options = [
  { value: 'remote', label: 'Remote', count: 1234 },
  { value: 'hybrid', label: 'Hybrid', count: 56 },
  { value: 'onsite', label: 'On-site', count: 7 },
];

describe('PillGroup', () => {
  it('states off, include and exclude programmatically', () => {
    render(PillGroup, { props: { options, include: ['remote'], exclude: ['onsite'], onToggle: () => {} } });
    expect(screen.getByRole('button', { name: /^Remote,\s?1,234$/ }).getAttribute('aria-pressed')).toBe('true');
    expect(screen.getByRole('button', { name: /^Hybrid,\s?56$/ }).getAttribute('aria-pressed')).toBe('false');
    expect(screen.getByRole('button', { name: /^On-site,\s?excluded,\s?7$/ }).getAttribute('aria-pressed')).toBe('true');
  });
});
