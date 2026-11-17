import { fireEvent, render, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import TailorWorkspace from './TailorWorkspace.svelte';

// The workspace is rendered for real; only its heavy children and the network are replaced.
// The chat's callbacks and the confirm dialogs are captured so a test can drive a turn and a
// download the way the children would, and the assertions read what the workspace sent to
// analytics — which is the wiring this change is about.
const mocks = vi.hoisted(() => ({
  chat: null as null | {
    onTurnStateChange: (active: boolean) => void;
    onDocumentEdited: () => void;
    onTurnComplete: (turn?: { completed: boolean }) => void | Promise<void>;
  },
  dialogs: [] as { title: string; onConfirm: () => void }[],
  track: vi.fn(),
  document: { summary: 'seed' } as Record<string, unknown>,
  style: null as null | { style: { font_size: number } },
  order: [] as string[],
  updateCv: (async () => {}) as () => Promise<unknown>,
}));

vi.mock('$app/state', () => ({
  page: { data: {}, url: new URL('http://localhost/tailor/acme-engineer?cv=cv-1'), state: {} },
}));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path, base: '', assets: '' }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));
vi.mock('#lib/analytics.js', () => ({ track: mocks.track }));
vi.mock('#lib/actions/fillViewport.js', () => ({ fillViewport: () => ({}) }));
vi.mock('#lib/api.js', () => {
  class ApiError extends Error {}
  return {
    ApiError,
    api: {
      cvPdfUrl: (id: string) => `/api/v1/me/cvs/${id}/pdf`,
      getJob: async () => ({ title: 'Engineer', company: 'Acme' }),
      getMatchAnalysis: async () => null,
      getCv: async () => ({
        id: 'cv-1',
        title: 'Acme CV',
        template_id: 'classic-ats',
        agent_session_id: 'session-1',
        tracer_links_enabled: false,
        document: mocks.document,
      }),
      tailorCv: async () => ({}),
      getPhoto: async () => ({ present: false }),
      getCvAtsDelta: async () => null,
      getCvJobMatch: async () => null,
      listCvRevisions: async () => [],
      listCvFonts: async () => [],
      updateCv: async () => {
        mocks.order.push('save');
        await mocks.updateCv();
        return {};
      },
    },
  };
});
vi.mock('#lib/ui/index.js', () => ({
  ConfirmDialog(_anchor: unknown, props: { title: string; onConfirm: () => void }) {
    mocks.dialogs.push(props);
  },
}));
vi.mock('#lib/assistant/AssistantChat.svelte', () => ({
  default(_anchor: unknown, props: NonNullable<typeof mocks.chat>) {
    mocks.chat = props;
  },
}));
// vi.mock is hoisted, so each presentation child is stubbed by its own call.
vi.mock('#lib/tailor/ArtifactPanel.svelte', () => ({ default() {} }));
vi.mock('#lib/components/cv/CliEditDialog.svelte', () => ({ default() {} }));
vi.mock('#lib/tailor/CvHtmlPreview.svelte', () => ({ default() {} }));
vi.mock('#lib/components/cv/CvSectionForm.svelte', () => ({ default() {} }));
vi.mock('#lib/components/ExperienceBankView.svelte', () => ({ default() {} }));
vi.mock('#lib/components/cv/MarginSettings.svelte', () => ({ default() {} }));
vi.mock('#lib/components/cv/TracerLinksSettings.svelte', () => ({ default() {} }));
// The Design tab's control writes through the workspace's bound document, so a test can type.
vi.mock('#lib/components/cv/StyleSettings.svelte', () => ({
  default(_anchor: unknown, props: { style: { font_size: number } }) {
    mocks.style = props;
  },
}));
vi.mock('#lib/tailor/TemplateGallery.svelte', () => ({ default() {} }));
vi.mock('#lib/components/AccountNavRail.svelte', () => ({ default() {} }));
vi.mock('./LeftPanelBar.svelte', () => ({ default() {} }));
vi.mock('./PanelSplitter.svelte', () => ({ default() {} }));
vi.mock('./TailorMobileTabs.svelte', () => ({ default() {} }));

const savedPdf = vi.fn();
const pdfResponse = (body = '%PDF-1.7', type = 'application/pdf') =>
  new Response(body, { status: 200, headers: { 'content-type': type } });

beforeEach(() => {
  mocks.chat = null;
  mocks.style = null;
  mocks.order.length = 0;
  mocks.updateCv = async () => {};
  mocks.dialogs.length = 0;
  mocks.document = { summary: 'seed' };
  mocks.track.mockClear();
  localStorage.clear();
  URL.createObjectURL = vi.fn(() => 'blob:pdf');
  URL.revokeObjectURL = vi.fn();
  savedPdf.mockClear();
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
    savedPdf(this.download);
  });
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function chatProps() {
  if (!mocks.chat) throw new Error('the chat was not mounted');
  return mocks.chat;
}

const outcomeCalls = () =>
  mocks.track.mock.calls.filter(([name]) => name === 'tailor_result' || name === 'cv_pdf_export');

async function open() {
  const view = render(TailorWorkspace, { slug: 'acme-engineer' });
  await waitFor(() => expect(mocks.chat).not.toBeNull());
  // Let the post-load accessory reads settle so the first assertion sees a quiet workspace.
  await new Promise((resolve) => setTimeout(resolve, 0));
  return view;
}

/** One agent turn the way the chat reports it: start, a successful cv_edit, then its end. */
async function agentTurn(opts: { persisted: Record<string, unknown>; completed: boolean }) {
  const chat = chatProps();
  chat.onTurnStateChange(true);
  mocks.document = opts.persisted;
  chat.onDocumentEdited();
  await chat.onTurnComplete({ completed: opts.completed });
}

it('reports nothing for a CV that was only seeded and opened', async () => {
  await open();
  expect(outcomeCalls()).toEqual([]);
});

it('reports a tailoring result once a completed turn left a changed document on the server', async () => {
  await open();
  await agentTurn({ persisted: { summary: 'Rewritten for Acme' }, completed: true });
  expect(outcomeCalls()).toEqual([['tailor_result', { slug: 'acme-engineer' }]]);
});

it('does not report a completed turn whose edit left the document as it was', async () => {
  await open();
  await agentTurn({ persisted: { summary: 'seed' }, completed: true });
  expect(outcomeCalls()).toEqual([]);
});

it('does not report a turn that was cut short or failed, even with edits persisted', async () => {
  await open();
  await agentTurn({ persisted: { summary: 'half way' }, completed: false });
  expect(outcomeCalls()).toEqual([]);
});

it('reports a duplicate turn-complete callback and a second turn only once', async () => {
  await open();
  await agentTurn({ persisted: { summary: 'one' }, completed: true });
  await chatProps().onTurnComplete({ completed: true });
  await agentTurn({ persisted: { summary: 'two' }, completed: true });
  expect(outcomeCalls()).toHaveLength(1);
});

it('does not report the same CV again when the workspace is remounted', async () => {
  const first = await open();
  await agentTurn({ persisted: { summary: 'one' }, completed: true });
  first.unmount();
  mocks.chat = null;
  await open();
  await agentTurn({ persisted: { summary: 'two' }, completed: true });
  expect(outcomeCalls()).toHaveLength(1);
});

it('reports a PDF export only after the file arrived and was handed to the browser', async () => {
  let deliver!: (response: Response) => void;
  vi.stubGlobal(
    'fetch',
    vi.fn(() => new Promise<Response>((resolve) => (deliver = resolve))),
  );
  const { getByText } = await open();

  await fireEvent.click(getByText('Download PDF'));
  expect(outcomeCalls()).toEqual([]); // the click alone is not a result

  deliver(pdfResponse());
  await waitFor(() => expect(outcomeCalls()).toEqual([['cv_pdf_export', { kind: 'tailored', slug: 'acme-engineer' }]]));
  expect(savedPdf).toHaveBeenCalledWith('cv.pdf');
});

it('reports nothing and says so when the PDF request fails', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('boom', { status: 500 })));
  const { getByText, findByRole } = await open();

  await fireEvent.click(getByText('Download PDF'));

  expect((await findByRole('alert')).textContent).toMatch(/could not download/i);
  expect(outcomeCalls()).toEqual([]);
  expect(savedPdf).not.toHaveBeenCalled();
});

it('reports nothing when the PDF request never gets an answer', async () => {
  vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('network down')));
  const { getByText, findByRole } = await open();

  await fireEvent.click(getByText('Download PDF'));

  await findByRole('alert');
  expect(outcomeCalls()).toEqual([]);
});

it('does not report a 200 that is not a PDF, such as a sign-in page after the session expired', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(pdfResponse('<html>Sign in</html>', 'text/html')));
  const { getByText, findByRole } = await open();

  await fireEvent.click(getByText('Download PDF'));

  await findByRole('alert');
  expect(outcomeCalls()).toEqual([]);
  expect(savedPdf).not.toHaveBeenCalled();
});

it('does not report an empty PDF body', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(pdfResponse('')));
  const { getByText, findByRole } = await open();

  await fireEvent.click(getByText('Download PDF'));

  await findByRole('alert');
  expect(outcomeCalls()).toEqual([]);
});

it('saves an edit still waiting on the autosave timer before it fetches the PDF', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => {
      mocks.order.push('pdf');
      return pdfResponse();
    }),
  );
  const { getByText } = await open();
  if (!mocks.style) throw new Error('the style control was not rendered');
  mocks.style.style = { ...mocks.style.style, font_size: 12 };
  await tick();

  await fireEvent.click(getByText('Download PDF'));

  await waitFor(() => expect(outcomeCalls()).toHaveLength(1));
  expect(mocks.order).toEqual(['save', 'pdf']);
});

it('does not export twice while one download is still in flight', async () => {
  let deliver!: (response: Response) => void;
  const fetcher = vi.fn(() => new Promise<Response>((resolve) => (deliver = resolve)));
  vi.stubGlobal('fetch', fetcher);
  const { getByText } = await open();

  await fireEvent.click(getByText('Download PDF'));
  await fireEvent.click(getByText('Download PDF'));
  deliver(pdfResponse());

  await waitFor(() => expect(outcomeCalls()).toHaveLength(1));
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it('holds a PDF with unfilled gaps behind the confirmation and reports only after confirming', async () => {
  mocks.document = { summary: 'Delivered [result] at Acme' };
  const fetcher = vi.fn().mockResolvedValue(pdfResponse());
  vi.stubGlobal('fetch', fetcher);
  const { getByText } = await open();

  await fireEvent.click(getByText('Download PDF'));
  expect(fetcher).not.toHaveBeenCalled();
  expect(outcomeCalls()).toEqual([]);

  const gaps = mocks.dialogs.find((d) => d.title.includes('gaps'));
  if (!gaps) throw new Error('the gaps dialog was not rendered');
  gaps.onConfirm();
  await waitFor(() => expect(outcomeCalls()).toHaveLength(1));
});

/** Types into the Design tab and lets the 800ms autosave timer fire, so the save is in flight. */
async function typeAndFireAutosave() {
  if (!mocks.style) throw new Error('the style control was not rendered');
  mocks.style.style = { ...mocks.style.style, font_size: 12 };
  await tick();
  await waitFor(() => expect(mocks.order).toEqual(['save']), { timeout: 3000 });
}

it('waits for a save already in flight before it fetches the PDF', async () => {
  let finish!: () => void;
  mocks.updateCv = () => new Promise<void>((resolve) => (finish = resolve));
  const fetcher = vi.fn(async () => pdfResponse());
  vi.stubGlobal('fetch', fetcher);
  const { getByText } = await open();
  await typeAndFireAutosave();

  await fireEvent.click(getByText('Download PDF'));
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(fetcher).not.toHaveBeenCalled();

  finish();
  await waitFor(() => expect(outcomeCalls()).toHaveLength(1));
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it('stops the export with its error when the save it waited on failed', async () => {
  mocks.updateCv = async () => {
    throw new Error('save failed');
  };
  const fetcher = vi.fn(async () => pdfResponse());
  vi.stubGlobal('fetch', fetcher);
  const { getByText, findAllByRole } = await open();
  await typeAndFireAutosave();

  await fireEvent.click(getByText('Download PDF'));

  const alerts = await findAllByRole('alert');
  expect(alerts.some((a) => /could not download/i.test(a.textContent ?? ''))).toBe(true);
  expect(fetcher).not.toHaveBeenCalled();
  expect(savedPdf).not.toHaveBeenCalled();
  expect(outcomeCalls()).toEqual([]);
});

it('retries the unsaved CV on the next export once saving works again', async () => {
  let fail = true;
  mocks.updateCv = async () => {
    if (fail) throw new Error('save failed');
  };
  const fetcher = vi.fn(async () => pdfResponse());
  vi.stubGlobal('fetch', fetcher);
  const { getByText, findAllByRole } = await open();
  await typeAndFireAutosave();
  await fireEvent.click(getByText('Download PDF'));
  await findAllByRole('alert');
  expect(fetcher).not.toHaveBeenCalled();

  fail = false;
  await fireEvent.click(getByText('Download PDF'));
  await waitFor(() => expect(outcomeCalls()).toHaveLength(1));
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it('starts the next autosave only after the one in flight has landed', async () => {
  let finish!: () => void;
  mocks.updateCv = () => new Promise<void>((resolve) => (finish = resolve));
  await open();
  await typeAndFireAutosave();

  // A second edit whose timer fires while the first save is still on the wire.
  if (!mocks.style) throw new Error('the style control was not rendered');
  mocks.style.style = { ...mocks.style.style, font_size: 13 };
  await tick();
  await new Promise((resolve) => setTimeout(resolve, 1000));
  expect(mocks.order).toEqual(['save']);

  mocks.updateCv = async () => {};
  finish();
  await waitFor(() => expect(mocks.order).toEqual(['save', 'save']));
});
