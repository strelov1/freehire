import { render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import Breadcrumbs from './breadcrumbs.svelte';
import { must } from './test-utils';

describe('Breadcrumbs', () => {
  it('renders every hub item as a link and the current item as unlinked text', () => {
    const { getByText } = render(Breadcrumbs, {
      items: [{ name: 'Talent Network', href: '/talent' }, { name: 'DevOps Engineer' }],
    });

    const hub = getByText('Talent Network');
    expect(hub.tagName).toBe('A');
    expect(hub.getAttribute('href')).toBe('/talent');

    const current = getByText('DevOps Engineer');
    expect(current.tagName).toBe('SPAN');
    expect(current.getAttribute('aria-current')).toBe('page');
  });

  it('separates items with a chevron but carries none before the first', () => {
    const { container } = render(Breadcrumbs, {
      items: [{ name: 'Jobs', href: '/jobs' }, { name: 'Senior Backend Engineer' }],
    });

    expect(container.querySelectorAll('svg')).toHaveLength(1);
  });

  it('names the nav for assistive tech', () => {
    const { getByRole } = render(Breadcrumbs, { items: [{ name: 'Companies', href: '/companies' }, { name: 'Acme' }] });

    expect(getByRole('navigation', { name: 'Breadcrumb' })).toBeTruthy();
  });

  it('renders two items sharing the same name without a keyed-each collision', () => {
    // A job titled exactly "QA", filed under the QA category, produces this shape —
    // keying the each block by name (instead of position) would throw or misrender.
    const { getAllByText } = render(Breadcrumbs, {
      items: [{ name: 'Jobs', href: '/jobs' }, { name: 'QA', href: '/jobs?category=qa' }, { name: 'QA' }],
    });

    const matches = getAllByText('QA');
    expect(matches).toHaveLength(2);
    expect(must(matches[0]).tagName).toBe('A');
    expect(must(matches[1]).tagName).toBe('SPAN');
  });
});
