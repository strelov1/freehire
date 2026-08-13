import { render } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import LoadMore from './load-more.svelte';

describe('LoadMore', () => {
  it('shows a label the caller can click', () => {
    const onclick = vi.fn();
    const { getByRole } = render(LoadMore, { loading: false, onclick });

    const button = getByRole('button');
    expect(button.textContent?.trim()).toBe('Load more');
    expect((button as HTMLButtonElement).disabled).toBe(false);

    button.click();
    expect(onclick).toHaveBeenCalledOnce();
  });

  it('swaps to a loading label and disables the button', () => {
    const { getByRole } = render(LoadMore, { loading: true, onclick: vi.fn() });

    const button = getByRole('button') as HTMLButtonElement;
    expect(button.textContent?.trim()).toBe('Loading…');
    expect(button.disabled).toBe(true);
  });

  it('shows no error line by default', () => {
    const { queryByText } = render(LoadMore, { loading: false, onclick: vi.fn() });

    expect(queryByText("Couldn't load more. Try again.")).toBeNull();
  });

  it('shows the error line when asked, without disabling the button', () => {
    const { getByRole, getByText } = render(LoadMore, { loading: false, error: true, onclick: vi.fn() });

    expect(getByText("Couldn't load more. Try again.")).toBeTruthy();
    expect((getByRole('button') as HTMLButtonElement).disabled).toBe(false);
  });
});
