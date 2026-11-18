import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import BackerBadge from './BackerBadge.svelte';

describe('BackerBadge overflow', () => {
  it('folds backers past max into a disclosure that links each one', () => {
    const { container } = render(BackerBadge, {
      collections: ['yc', 'techstars', 'sequoia', 'accel'],
      withLabel: true,
      max: 2,
    });

    const details = container.querySelector('details');
    expect(details?.querySelector('summary')?.textContent).toContain('+2');
    const hrefs = [...(details?.querySelectorAll('a') ?? [])].map((a) => a.getAttribute('href'));
    expect(hrefs).toHaveLength(2);
    expect(hrefs.every((h) => h?.startsWith('/collections/'))).toBe(true);
  });

  it('renders no disclosure when everything fits', () => {
    const { container } = render(BackerBadge, { collections: ['yc'], max: 3 });
    expect(container.querySelector('details')).toBeNull();
  });
});
