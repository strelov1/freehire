import { render, waitFor } from '@testing-library/svelte';
import { afterEach, expect, it, vi } from 'vitest';
import AssistantChat from './AssistantChat.svelte';

// The composer's props, captured so a test can send a message the way the visitor does.
const composer = vi.hoisted(() => ({ props: null as null | { onSubmit: (text: string) => void } }));

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
vi.mock('#lib/assistant/Composer.svelte', () => ({
  default(_anchor: unknown, props: { onSubmit: (text: string) => void }) {
    composer.props = props;
  },
}));
vi.mock('#lib/assistant/ThinkingIndicator.svelte', () => ({ default() {} }));
vi.mock('@lucide/svelte', () => ({
  AlertTriangle() {},
  ArrowDown() {},
  FileText() {},
  MessagesSquare() {},
  PanelLeft() {},
  Phone() {},
  Plus() {},
  Rocket() {},
  Trash2() {},
  X() {},
}));

afterEach(() => vi.unstubAllGlobals());

it('does not start an opening after its host replaced a still-loading conversation', async () => {
  let resolveSession!: (response: Response) => void;
  const pending = new Promise<Response>((resolve) => {
    resolveSession = resolve;
  });
  const fetcher = vi.fn((url: string) =>
    String(url).endsWith('/sessions') ? Promise.resolve(Response.json({ data: [] })) : pending,
  );
  vi.stubGlobal('fetch', fetcher);
  const { unmount } = render(AssistantChat, {
    session: '7fe956bc-cae6-4cc8-b527-a0d76cb5bc14',
    kickoff: 'Shortlist this search',
    showSessionRail: false,
  });
  await waitFor(() =>
    expect(fetcher.mock.calls.some(([url]) => String(url).endsWith('/7fe956bc-cae6-4cc8-b527-a0d76cb5bc14'))).toBe(
      true,
    ),
  );
  unmount();
  resolveSession(
    Response.json({
      data: { session: { id: '7fe956bc-cae6-4cc8-b527-a0d76cb5bc14', preset: 'chat', label: '' }, messages: [] },
    }),
  );
  await pending;
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(fetcher.mock.calls.some(([url]) => String(url).endsWith('/messages'))).toBe(false);
});

it('warns that a support answer may be cut off when its stream drops', async () => {
  // A support session cannot be re-read through the generic transcript route, so a dropped
  // stream loses the rest of the answer; the chat must say so rather than stop silently.
  const dropped = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.error(new Error('network went away'));
    },
  });
  const fetcher = vi.fn((url: string) => {
    if (String(url).endsWith('/support/sessions')) {
      return Promise.resolve(Response.json({ data: { id: 'support-1', preset: 'support', label: '' } }));
    }
    if (String(url).endsWith('/support/sessions/support-1/messages')) {
      return Promise.resolve(new Response(dropped, { status: 200, headers: { 'content-type': 'text/event-stream' } }));
    }
    return Promise.resolve(Response.json({ data: [] }));
  });
  vi.stubGlobal('fetch', fetcher);
  const { findByText } = render(AssistantChat, {
    preset: 'support',
    kickoff: 'Where is my refund?',
    showSessionRail: false,
  });
  expect(await findByText(/this answer may be cut off/)).toBeTruthy();
});

it('keeps the cut-off warning up through a queued message and closes the cut answer', async () => {
  // The visitor queued a message behind the turn whose stream then dropped. The queue drains
  // the moment that turn ends, so a warning cleared by the next turn starting would vanish
  // before it could be read.
  const encoder = new TextEncoder();
  const frame = (event: object) => encoder.encode(`data: ${JSON.stringify(event)}\n\n`);
  let first!: ReadableStreamDefaultController<Uint8Array>;
  const firstStream = new ReadableStream<Uint8Array>({
    start(controller) {
      first = controller;
    },
  });
  const secondStream = new ReadableStream<Uint8Array>({});
  const answers = [firstStream, secondStream];
  const fetcher = vi.fn((url: string) => {
    if (String(url).endsWith('/support/sessions')) {
      return Promise.resolve(Response.json({ data: { id: 'support-1', preset: 'support', label: '' } }));
    }
    if (String(url).endsWith('/support/sessions/support-1/messages')) {
      return Promise.resolve(
        new Response(answers.shift(), { status: 200, headers: { 'content-type': 'text/event-stream' } }),
      );
    }
    return Promise.resolve(Response.json({ data: [] }));
  });
  vi.stubGlobal('fetch', fetcher);
  const messageCalls = () => fetcher.mock.calls.filter(([url]) => String(url).endsWith('/messages')).length;
  const { container, findByText, queryByText } = render(AssistantChat, {
    preset: 'support',
    kickoff: 'Where is my refund?',
    showSessionRail: false,
  });
  await waitFor(() => expect(messageCalls()).toBe(1));

  // A frame arrives before the drop, so there is an open answer to close.
  first.enqueue(frame({ type: 'assistant_thought', text: 'Looking up the refund policy' }));
  first.enqueue(frame({ type: 'assistant_text', text: 'Refunds take five days' }));
  expect(await findByText(/Refunds take five days/)).toBeTruthy();
  expect(container.querySelector('details')?.hasAttribute('open')).toBe(true);

  // The visitor queues a message behind the running turn, then the stream drops.
  composer.props?.onSubmit('And my invoice?');
  first.error(new Error('network went away'));

  // The queued message starts as the dropped turn ends; the warning outlives that.
  await waitFor(() => expect(messageCalls()).toBe(2));
  expect(await findByText(/this answer may be cut off/)).toBeTruthy();
  // What streamed stays on screen, closed rather than still streaming, and not marked failed.
  expect(await findByText(/Refunds take five days/)).toBeTruthy();
  expect(container.querySelector('details')?.hasAttribute('open')).toBe(false);
  expect(queryByText(/ended the turn with an error/)).toBeNull();

  // The visitor's own next message is what removes it.
  composer.props?.onSubmit('Never mind');
  await waitFor(() => expect(queryByText(/this answer may be cut off/)).toBeNull());
});

it('opens a first chat without creating a session, and creates it with the first message', async () => {
  // Minting a session on page open left most chat sessions empty: new accounts land here and
  // leave without typing. The row now waits for the first message.
  const id = '0c1d6c2e-58a4-4b43-9d1a-2f6c0d9b7a11';
  const fetcher = vi.fn((url: string, init?: RequestInit) => {
    const u = String(url);
    if (u.endsWith('/assistant/sessions') && init?.method === 'POST') {
      return Promise.resolve(Response.json({ data: { id, preset: 'chat', label: '' } }));
    }
    if (u.endsWith(`/assistant/sessions/${id}`)) {
      return Promise.resolve(Response.json({ data: { session: { id, preset: 'chat', label: '' }, messages: [] } }));
    }
    if (u.endsWith(`/assistant/sessions/${id}/messages`)) {
      // Held open: an ended stream sends a plain chat into its catch-up path, which is not under test.
      return Promise.resolve(
        new Response(new ReadableStream<Uint8Array>({}), {
          status: 200,
          headers: { 'content-type': 'text/event-stream' },
        }),
      );
    }
    return Promise.resolve(Response.json({ data: [] }));
  });
  vi.stubGlobal('fetch', fetcher);
  composer.props = null;
  const { unmount } = render(AssistantChat, {});
  await waitFor(() => expect(composer.props).not.toBeNull());
  await new Promise((resolve) => setTimeout(resolve, 20));
  const posted = () =>
    fetcher.mock.calls.filter(([u, init]) => String(u).endsWith('/assistant/sessions') && init?.method === 'POST');
  expect(posted()).toHaveLength(0);

  // Cast: TypeScript keeps the `null` assigned above and does not see the mock's write.
  (composer.props as { onSubmit: (text: string) => void } | null)?.onSubmit('Find me remote Go jobs');
  await waitFor(() => expect(fetcher.mock.calls.some(([u]) => String(u).endsWith(`/${id}/messages`))).toBe(true));
  expect(posted()).toHaveLength(1);
  unmount();
});

it('leaves the caller on the chat they opened while their first message was creating a session', async () => {
  const created = '5b2f7a40-1c9e-4d0a-8f61-3e7d2c9a4b10';
  const other = '9e8d7c6b-5a49-4382-9171-605f4e3d2c1b';
  let finishCreate!: (response: Response) => void;
  const fetcher = vi.fn((url: string, init?: RequestInit) => {
    const u = String(url);
    if (u.endsWith('/assistant/sessions') && init?.method === 'POST') {
      return new Promise<Response>((resolve) => {
        finishCreate = resolve;
      });
    }
    if (u.endsWith(`/assistant/sessions/${other}`)) {
      return Promise.resolve(
        Response.json({ data: { session: { id: other, preset: 'chat', label: 'Earlier' }, messages: [] } }),
      );
    }
    return Promise.resolve(Response.json({ data: [] }));
  });
  vi.stubGlobal('fetch', fetcher);
  composer.props = null;
  const { rerender, unmount } = render(AssistantChat, {});
  await waitFor(() => expect(composer.props).not.toBeNull());
  await new Promise((resolve) => setTimeout(resolve, 20));

  (composer.props as { onSubmit: (text: string) => void } | null)?.onSubmit('Find me remote Go jobs');
  await waitFor(() => expect(finishCreate).toBeTypeOf('function'));
  await rerender({ session: other });
  await waitFor(() => expect(fetcher.mock.calls.some(([u]) => String(u).endsWith(`/${other}`))).toBe(true));
  finishCreate(Response.json({ data: { id: created, preset: 'chat', label: '' } }));
  await new Promise((resolve) => setTimeout(resolve, 50));

  // Neither opened nor sent to: the caller stays on the chat they picked.
  expect(fetcher.mock.calls.some(([u]) => String(u).includes(`/${created}`))).toBe(false);
  unmount();
});
