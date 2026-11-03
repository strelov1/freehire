import { render, waitFor } from '@testing-library/svelte';
import { afterEach, expect, it, vi } from 'vitest';
import AssistantChat from './AssistantChat.svelte';

vi.mock('$app/state', () => ({ page: { data: { locale: 'en' }, url: new URL('http://localhost/jobs'), state: {} } }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('#lib/analytics.js', () => ({ track() {} }));
// Keep session orchestration real; these presentation children do not affect startup.
vi.mock('#lib/ui/index.js', () => ({ Button() {}, Chip() {}, ConfirmDialog() {} }));
vi.mock('#lib/assistant/VoiceCall.svelte', () => ({ default() {} }));
vi.mock('#lib/assistant/JobDeck.svelte', () => ({ default() {} }));
vi.mock('#lib/assistant/SessionRail.svelte', () => ({ default() {} }));
vi.mock('#lib/assistant/ToolGroupList.svelte', () => ({ default() {} }));
vi.mock('#lib/assistant/Composer.svelte', () => ({ default() {} }));
vi.mock('#lib/assistant/ThinkingIndicator.svelte', () => ({ default() {} }));
vi.mock('@lucide/svelte', () => ({
  AlertTriangle() {}, ArrowDown() {}, FileText() {}, MessagesSquare() {}, PanelLeft() {},
  Phone() {}, Plus() {}, Rocket() {}, Trash2() {}, X() {},
}));

afterEach(() => vi.unstubAllGlobals());

it('does not start an opening after its host replaced a still-loading conversation', async () => {
  let resolveSession!: (response: Response) => void;
  const pending = new Promise<Response>((resolve) => { resolveSession = resolve; });
  const fetcher = vi.fn((url: string) => String(url).endsWith('/sessions')
    ? Promise.resolve(Response.json({ data: [] }))
    : pending);
  vi.stubGlobal('fetch', fetcher);
  const { unmount } = render(AssistantChat, {
    session: '7fe956bc-cae6-4cc8-b527-a0d76cb5bc14', kickoff: 'Shortlist this search', showSessionRail: false,
  });
  await waitFor(() => expect(fetcher.mock.calls.some(([url]) => String(url).endsWith('/7fe956bc-cae6-4cc8-b527-a0d76cb5bc14'))).toBe(true));
  unmount();
  resolveSession(Response.json({ data: { session: { id: '7fe956bc-cae6-4cc8-b527-a0d76cb5bc14', preset: 'chat', label: '' }, messages: [] } }));
  await pending;
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(fetcher.mock.calls.some(([url]) => String(url).endsWith('/messages'))).toBe(false);
});
