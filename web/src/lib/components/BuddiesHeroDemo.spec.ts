import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
vi.mock('$app/paths', () => ({ resolve: (p: string) => p, base: '', assets: '' }));

import BuddiesHeroDemo from './BuddiesHeroDemo.svelte';

describe('BuddiesHeroDemo', () => {
  it('draws the demo posts with the real feed post, out of reach of keyboard and pointer', () => {
    const { container } = render(BuddiesHeroDemo);
    expect(container.querySelectorAll('article')).toHaveLength(2);
    expect(container.querySelector('[inert]')?.contains(container.querySelector('article'))).toBe(true);
    expect(screen.getByText(/An example feed/)).toBeTruthy();
  });
});
