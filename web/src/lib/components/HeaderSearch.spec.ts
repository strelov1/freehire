import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import HeaderSearch from './HeaderSearch.svelte';

// The suggestion panel's ARIA shape: every child of the listbox is either an option or
// a presentational caption, and an option is itself the interactive element — no
// focusable button nested inside it — while the input keeps the keyboard.

const { goto } = vi.hoisted(() => ({ goto: vi.fn() }));

vi.mock('$app/navigation', () => ({
  goto,
  afterNavigate: () => {},
  beforeNavigate: () => {},
  onNavigate: () => {},
  replaceState: () => {},
  pushState: () => {},
}));
vi.mock('$app/state', () => ({
  page: { data: {}, url: new URL('http://localhost/'), params: {} },
}));
// `$app/env` has no stub in the components project, so the two modules HeaderSearch
// reaches it through are replaced; this panel test drives neither.
// Plain functions stand in for the classes: something only has to be extendable.
function Stub() {}
vi.mock('#lib/urlSynced.svelte.js', () => ({ UrlSyncedState: Stub, syncOnNavigation: () => {} }));
vi.mock('#lib/userResource.svelte.js', () => ({ resetUserStores: () => {}, UserResource: Stub, SlugSet: Stub }));
vi.mock('#lib/auth.svelte.js', () => ({ isAuthenticated: () => false }));
vi.mock('#lib/api.js', () => ({
  api: {
    suggest: vi.fn().mockResolvedValue([]),
    searchJobs: vi.fn().mockResolvedValue({ items: [], hasMore: false }),
    listCompanies: vi.fn().mockResolvedValue({ items: [], hasMore: false }),
    facetCounts: vi.fn().mockResolvedValue({}),
  },
}));

beforeEach(() => {
  goto.mockReset();
});

async function openWithText(text: string) {
  render(HeaderSearch, { props: { placeholder: 'Search jobs', label: 'Search jobs' } });
  const input = screen.getByRole('combobox', { name: 'Search jobs' });
  await fireEvent.focus(input);
  await fireEvent.input(input, { target: { value: text } });
  const listbox = await screen.findByRole('listbox', { name: 'Search suggestions' });
  return { input, listbox };
}

describe('HeaderSearch suggestion panel', () => {
  it('puts only options and presentational captions in the listbox, with no nested buttons', async () => {
    const { listbox } = await openWithText('react');

    for (const child of Array.from(listbox.children)) {
      expect(['option', 'presentation']).toContain(child.getAttribute('role'));
    }
    const options = within(listbox).getAllByRole('option');
    expect(options.length).toBeGreaterThan(0);
    for (const o of options) {
      expect(o.getAttribute('tabindex')).toBe('-1');
      expect(o.querySelector('button, a, input')).toBeNull();
    }
  });

  it('keeps the keyboard on the input: ArrowDown highlights through aria-activedescendant', async () => {
    const { input, listbox } = await openWithText('react');

    await fireEvent.keyDown(input, { key: 'ArrowDown' });
    const first = within(listbox).getAllByRole('option')[0] as HTMLElement;
    expect(input.getAttribute('aria-activedescendant')).toBe(first.id);
    expect(first.getAttribute('aria-selected')).toBe('true');
  });

  it('closes the panel on Escape', async () => {
    const { input } = await openWithText('react');

    await fireEvent.keyDown(input, { key: 'Escape' });
    expect(screen.queryByRole('listbox')).toBeNull();
  });

  it('activates an option on click', async () => {
    const { listbox } = await openWithText('react');

    const textRow = within(listbox).getByRole('option', { name: /as text/ });
    await fireEvent.click(textRow);
    expect(goto).toHaveBeenCalled();
  });
});

// The page places the caret in the homepage search on wide screens. That focus is not one
// the visitor moved, so the ring stays off until they press a key or the pointer inside;
// then the full focus ring returns.
describe('HeaderSearch autofocus ring', () => {
  it('draws no ring for focus the page placed, and the ring once the visitor acts', async () => {
    vi.stubGlobal('matchMedia', (q: string) => ({
      matches: q.includes('min-width'),
      addEventListener() {},
      removeEventListener() {},
    }));
    render(HeaderSearch, { props: { placeholder: 'Search jobs', label: 'Search jobs', autofocus: true } });
    const input = screen.getByRole('combobox', { name: 'Search jobs' });
    const frame = () => input.closest('div.flex.items-center') as HTMLElement;

    await vi.waitFor(() => expect(document.activeElement).toBe(input));
    expect(frame().className).not.toContain('focus-within:ring-2');

    await fireEvent.keyDown(input, { key: 'a' });
    expect(frame().className).toContain('focus-within:ring-2');
    vi.unstubAllGlobals();
  });
});
