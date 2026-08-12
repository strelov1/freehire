import { fireEvent, render } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import Pagination from './pagination.svelte';
import { must } from './test-utils';

function setup(props: { page?: number; total: number; perPage?: number }) {
  const { container, getByLabelText } = render(Pagination, props);
  return {
    label: () => must(must(container.querySelector('span')).textContent).replace(/\s+/g, ' ').trim(),
    prev: getByLabelText('Previous page') as HTMLButtonElement,
    next: getByLabelText('Next page') as HTMLButtonElement,
  };
}

describe('Pagination', () => {
  it('counts the pages from the total and the page size', () => {
    expect(setup({ total: 50, perPage: 20 }).label()).toBe('Page 1 of 3');
  });

  // The regression: any narrowing filter shrinks `total`, which used to strand
  // `page` past the end — "Page 7 of 3", Next disabled, nothing to show.
  it('clamps a page that the shrinking total left behind', () => {
    const { label, next } = setup({ page: 7, total: 50, perPage: 20 });

    expect(label()).toBe('Page 3 of 3');
    expect(next.disabled).toBe(true);
  });

  it('clamps a page below the first one', () => {
    expect(setup({ page: 0, total: 50, perPage: 20 }).label()).toBe('Page 1 of 3');
  });

  it('keeps one page when there is nothing to show', () => {
    const { label, prev, next } = setup({ total: 0 });

    expect(label()).toBe('Page 1 of 1');
    expect(prev.disabled).toBe(true);
    expect(next.disabled).toBe(true);
  });

  it('steps forward and back within range', async () => {
    const { label, prev, next } = setup({ total: 50, perPage: 20 });

    expect(prev.disabled).toBe(true);

    await fireEvent.click(next);
    expect(label()).toBe('Page 2 of 3');
    expect(prev.disabled).toBe(false);

    await fireEvent.click(prev);
    expect(label()).toBe('Page 1 of 3');
  });

  it('stops at the last page', async () => {
    const { label, next } = setup({ page: 3, total: 50, perPage: 20 });

    await fireEvent.click(next);

    expect(label()).toBe('Page 3 of 3');
  });
});
