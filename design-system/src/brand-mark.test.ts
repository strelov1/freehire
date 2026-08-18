import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import BrandMark from './brand-mark.svelte';
import { must } from './test-utils';

describe('BrandMark', () => {
  it('renders the path with the brand hex as its fill', () => {
    const { container } = render(BrandMark, { path: 'M0 0h24v24H0z', hex: '61DAFB', title: 'React' });

    const svg = must(container.querySelector('svg'));
    expect(svg.getAttribute('role')).toBe('img');
    expect(svg.getAttribute('aria-label')).toBe('React');
    const path = must(container.querySelector('path'));
    expect(path.getAttribute('d')).toBe('M0 0h24v24H0z');
    expect(path.getAttribute('fill')).toBe('#61DAFB');
  });

  it('defaults to size-4 and accepts a class override', () => {
    const { container } = render(BrandMark, { path: 'M0 0z', hex: '000', title: 'X' });
    expect(must(container.querySelector('svg')).getAttribute('class')).toBe('size-4');

    const { container: sized } = render(BrandMark, { path: 'M0 0z', hex: '000', title: 'X', class: 'size-3' });
    expect(must(sized.querySelector('svg')).getAttribute('class')).toBe('size-3');
  });
});
