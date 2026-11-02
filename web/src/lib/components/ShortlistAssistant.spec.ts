import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ShortlistAssistant from './ShortlistAssistant.svelte';

const { goto } = vi.hoisted(() => ({ goto: vi.fn() }));
vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/jobs?q=stale') } }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ goto }));
// The host owns request capture and retries; conversation rendering is covered by the chat.
vi.mock('#lib/assistant/AssistantChat.svelte', () => ({ default() {} }));
vi.mock('@lucide/svelte', () => ({ MessageSquareText() {}, ExternalLink() {}, X() {} }));
vi.mock('#lib/ui/index.js', async () => {
  const actual = await vi.importActual<typeof import('#lib/ui/index.js')>('#lib/ui/index.js');
  return { Button: actual.Button, lockScroll: actual.lockScroll, unlockScroll: actual.unlockScroll };
});

beforeEach(() => {
  sessionStorage.clear();
  goto.mockClear();
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {});
});
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe('shortlist entry', () => {
  it('does not create a conversation until asked', () => {
    const fetcher = vi.fn();
    vi.stubGlobal('fetch', fetcher);
    render(ShortlistAssistant, { params: new URLSearchParams('q=go'), userId: 7 });
    expect(screen.getByRole('button', { name: 'Help me shortlist' })).toBeTruthy();
    expect(fetcher).not.toHaveBeenCalled();
  });

  it('returns from sign-in to the live filters rather than the stale route URL', async () => {
    render(ShortlistAssistant, { params: new URLSearchParams('q=go&salary_min=80000'), userId: null });
    await fireEvent.click(screen.getByRole('button', { name: 'Help me shortlist' }));
    const destination = new URL(goto.mock.calls[0]?.[0] as string, 'http://localhost');
    expect(destination.pathname).toBe('/signin');
    expect(destination.searchParams.get('returnTo')).toBe('/jobs?q=go&salary_min=80000');
  });

  it('retries a failed startup with the same request id and captured search', async () => {
    const fetcher = vi.fn().mockImplementation(() => Promise.resolve(Response.json({ error: 'Unsupported search condition: location' }, { status: 400 })));
    vi.stubGlobal('fetch', fetcher);
    const { rerender } = render(ShortlistAssistant, { params: new URLSearchParams('q=go&page=4'), userId: 7 });
    await fireEvent.click(screen.getByRole('button', { name: 'Help me shortlist' }));
    await screen.findByRole('button', { name: 'Retry' });
    expect(screen.getByRole('alert').textContent).toContain('Unsupported search condition: location');
    await rerender({ params: new URLSearchParams('q=rust'), userId: 7 });
    await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
    expect(fetcher.mock.calls[0]?.[1].body).toBe(fetcher.mock.calls[1]?.[1].body);
    expect(JSON.parse(fetcher.mock.calls[1]?.[1].body)).toMatchObject({ search_query: 'q=go' });
  });

  it('keeps its captured search across close, reopen, and page filter edits', async () => {
    const fetcher = vi.fn(async (_url: string, init: RequestInit) => {
      const body = JSON.parse(String(init.body));
      return Response.json({ data: { id: body.request_id, preset: 'chat', label: '', search_context: { query: body.search_query, brief: '' } } });
    });
    vi.stubGlobal('fetch', fetcher);
    const { rerender } = render(ShortlistAssistant, { params: new URLSearchParams('q=go'), userId: 7 });
    await fireEvent.click(screen.getByRole('button', { name: 'Help me shortlist' }));
    await waitFor(() => expect(screen.queryByText('Opening your conversation…')).toBeNull());
    expect(screen.getByRole('link', { name: 'View search and filters' }).getAttribute('href')).toBe('/jobs?q=go');
    await fireEvent.click(screen.getByRole('button', { name: 'Close assistant' }));
    await rerender({ params: new URLSearchParams('q=rust'), userId: 7 });
    await fireEvent.click(screen.getByRole('button', { name: 'Help me shortlist' }));
    await screen.findByText('Your page filters have changed. This conversation keeps its original search.');
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('link', { name: 'View search and filters' }).getAttribute('href')).toBe('/jobs?q=go');
    await waitFor(() => expect((screen.getByRole('button', { name: 'Start from current filters' }) as HTMLButtonElement).disabled).toBe(false));
    await fireEvent.click(screen.getByRole('button', { name: 'Start from current filters' }));
    await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(2));
    const first = JSON.parse(String(fetcher.mock.calls[0]?.[1].body));
    const next = JSON.parse(String(fetcher.mock.calls[1]?.[1].body));
    expect(next.search_query).toBe('q=rust');
    expect(next.request_id).not.toBe(first.request_id);
  });

});
