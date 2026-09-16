import {
  emptySnapshot,
  type RuntimeMessage,
  type FramedField,
  type FramedUpload,
  type LabelFill,
  type FillOutcome,
  type ComboboxStep,
  type ComboboxReply,
  type RevealRequest,
} from '../lib/protocol';
import { mergeComboboxReplies, mergeFrameOutcomes } from '../lib/tools/executor';
import { fillsForFrame } from '../lib/form';
import { getToken } from '../lib/auth';
import { getTailoredCVForJob, getCVPdfBytes } from '../lib/freehire';
import {
  uploadInputExpression,
  pdfDataUrl,
  resolveDownloadedPath,
  pickAttachableUpload,
  classifyAttachError,
  type DownloadsAPI,
} from '../lib/tools/attachCv';

/**
 * Service worker. Three jobs, all thin:
 *  - open the side panel when the toolbar icon is clicked;
 *  - relay a snapshot request from the panel to the active tab's content
 *    script (the panel can't message a content script directly);
 *  - fan the browser-tool primitives out across the tab's frames, since an
 *    apply form is often served from an ATS iframe. Only this side knows the
 *    frame ids, so it is also what tags the fields it reads back.
 */
export default defineBackground(() => {
  browser.sidePanel
    ?.setPanelBehavior({ openPanelOnActionClick: true })
    .catch((err) => console.error('setPanelBehavior failed', err));

  browser.runtime.onMessage.addListener((message: RuntimeMessage) => {
    switch (message.kind) {
      case 'GET_PAGE_SNAPSHOT':
        return readTopFrameSnapshot();
      case 'GET_FRAMED_FORM':
        return readFramedForm();
      case 'FILL_BY_LABEL':
        return fillAcrossFrames(message.fills, message.reveal);
      case 'REVEAL_FIELD':
        return revealAcrossFrames(message.request);
      case 'COMBOBOX_STEP':
        return comboboxAcrossFrames(message.step);
      case 'ATTACH_TAILORED_CV':
        return attachTailoredCV(message.jobSlug);
      default:
        return undefined;
    }
  });
});

/** The tab's top document — frame 0 in every Chromium tab. */
const TOP_FRAME = 0;

/**
 * Reads the page the user is looking at, which is the TOP document — so the
 * snapshot is asked of that frame by id.
 *
 * Nothing here may address the active tab without naming a frame:
 * `tabs.sendMessage` with no `frameId` reaches every frame the content script
 * runs in and resolves with whichever answers first, which is reliably the
 * emptiest one. That cost us the match card once (an ad iframe answered, so the
 * card came out titled "This page") and the deterministic autofill again (a
 * Google Maps embed on a careers page won the race 10 times out of 10, and the
 * panel reported "no form fields on this page" while the form sat in frame 0).
 * Everything else fans out over `eachFrame` and folds the answers together.
 */
async function readTopFrameSnapshot(): Promise<RuntimeMessage> {
  const tabId = await activeTabId();
  if (tabId == null) return { kind: 'PAGE_SNAPSHOT', snapshot: emptySnapshot() };
  return browser.tabs.sendMessage(tabId, { kind: 'GET_PAGE_SNAPSHOT' } satisfies RuntimeMessage, {
    frameId: TOP_FRAME,
  });
}

/** Reads every frame of the active tab, tagging each observation with its frame. */
async function readFramedForm(): Promise<RuntimeMessage> {
  const fields: FramedField[] = [];
  const uploads: FramedUpload[] = [];
  await eachFrame({ kind: 'GET_FRAMED_FORM' }, (reply, frame) => {
    if (reply?.kind !== 'FORM') return;
    for (const field of reply.fields) fields.push({ ...field, frame });
    for (const upload of reply.uploads) uploads.push({ ...upload, frame });
  });
  return { kind: 'FRAMED_FORM', fields, uploads };
}

/**
 * Offers each frame only the fills addressed to it (`fillsForFrame`) and folds
 * the answers together. A fill naming a frame is withheld from every other one,
 * so a same-labeled control there cannot absorb it the way a page-wide
 * broadcast would; a fill naming none — a `fill_simple` tool call — still goes
 * to every frame, and a frame that does not hold it reports `not_found`.
 */
async function fillAcrossFrames(fills: LabelFill[], reveal?: boolean): Promise<RuntimeMessage> {
  const perFrame: FillOutcome[][] = [];
  await eachFrame(
    (frame) => ({ kind: 'FILL_BY_LABEL', fills: fillsForFrame(fills, frame), reveal }),
    (reply) => {
      if (reply?.kind === 'FILL_OUTCOMES') perFrame.push(reply.outcomes);
    },
  );
  return { kind: 'FILL_OUTCOMES', outcomes: mergeFrameOutcomes(perFrame) };
}

/**
 * Reveals one question. A request naming its frame goes to that frame alone —
 * two frames carrying the same label would otherwise both scroll and both take
 * the cursor. A request naming none (the agent's report carries labels only) is
 * offered to every frame, and the answers are folded with "some frame found it"
 * rather than by whichever replied first.
 */
async function revealAcrossFrames(request: RevealRequest): Promise<RuntimeMessage> {
  let found = false;
  await eachFrame(
    { kind: 'REVEAL_FIELD', request },
    (reply) => {
      if (reply?.kind === 'REVEAL_RESULT' && reply.found) found = true;
    },
    request.frame,
  );
  return { kind: 'REVEAL_RESULT', found };
}

/**
 * Offers one widget step to every frame. The widget lives in exactly one of
 * them, so every other frame answers `not_found` and the merge keeps the frame
 * that actually holds it.
 */
async function comboboxAcrossFrames(step: ComboboxStep): Promise<RuntimeMessage> {
  const replies: ComboboxReply[] = [];
  await eachFrame({ kind: 'COMBOBOX_STEP', step }, (reply) => {
    if (reply?.kind === 'COMBOBOX_REPLY') replies.push(reply.reply);
  });
  return { kind: 'COMBOBOX_REPLY', reply: mergeComboboxReplies(replies) };
}

/**
 * Sends a message to each injectable frame of the active tab, in parallel. The
 * message can be built per frame — `fillAcrossFrames` uses that to withhold a
 * frame-addressed fill from every frame but its own — or given fixed, for a
 * request every frame answers the same way.
 */
async function eachFrame(
  message: RuntimeMessage | ((frame: number) => RuntimeMessage),
  take: (reply: RuntimeMessage | undefined, frame: number) => void,
  /** Restricts the fan-out to one frame, for a request that names the frame it
   *  belongs to. Everything else still reaches every frame. */
  onlyFrame?: number,
): Promise<void> {
  const tabId = await activeTabId();
  if (tabId == null) return;
  const forFrame = typeof message === 'function' ? message : () => message;
  const frames = onlyFrame === undefined ? await frameIds(tabId) : [onlyFrame];
  await Promise.all(
    frames.map(async (frameId) => {
      try {
        take((await browser.tabs.sendMessage(tabId, forFrame(frameId), { frameId })) as RuntimeMessage, frameId);
      } catch {
        // No content script in that frame (about:blank, a restricted origin) —
        // it simply contributes nothing.
      }
    }),
  );
}

/**
 * The frames of a tab we can reach. Enumerated by injecting a no-op into all of
 * them: each InjectionResult carries its frameId, which gives us the list using
 * the `scripting` permission we already hold, without asking for `webNavigation`.
 */
async function frameIds(tabId: number): Promise<number[]> {
  try {
    const results = await browser.scripting.executeScript({
      target: { tabId, allFrames: true },
      func: () => 0,
    });
    return results.map((r) => r.frameId);
  } catch {
    return [0];
  }
}

async function activeTabId(): Promise<number | null> {
  const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
  return tab?.id ?? null;
}

/** How long a download may sit unresolved before `resolveDownloadedPath` gives up — bounds
 *  the "ask where to save each file" case (design.md, Risks), which this code cannot see. */
const DOWNLOAD_TIMEOUT_MS = 20_000;

/** The real `chrome.downloads` behind `resolveDownloadedPath`'s `DownloadsAPI` seam. */
const realDownloadsAPI: DownloadsAPI = {
  download: (options) => browser.downloads.download(options),
  search: (query) => browser.downloads.search(query),
  onChanged: {
    addListener: (cb) => browser.downloads.onChanged.addListener(cb),
    removeListener: (cb) => browser.downloads.onChanged.removeListener(cb),
  },
};

/** `Error#message` when there is one, else the value's own string form. */
function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/**
 * Attaches the caller's tailored CV for `jobSlug` to the current page's upload field.
 * See lib/tools/attachCv.ts and openspec/changes/extension-attach-tailored-cv/design.md
 * for why this needs the debugging protocol at all.
 */
async function attachTailoredCV(jobSlug: string): Promise<RuntimeMessage> {
  const fail = (error: string): RuntimeMessage => ({ kind: 'ATTACH_TAILORED_CV_RESULT', ok: false, error });

  const token = await getToken();
  if (!token) return fail('not signed in');

  const tabId = await activeTabId();
  if (tabId == null) return fail('no active tab to attach the file to');

  // Independent of each other — reading the page's frames and fetching the tailored CV
  // metadata share nothing, so there is no reason to pay their latency one after another.
  const [formReply, cv] = await Promise.all([readFramedForm(), getTailoredCVForJob(jobSlug, token)]);
  if (formReply.kind !== 'FRAMED_FORM') return fail('could not read this page to find the upload field');

  const pick = pickAttachableUpload(formReply.uploads);
  switch (pick.kind) {
    case 'none':
      return fail('no file upload field found on this page');
    case 'unreachable':
      return fail("can't reach this embedded form yet — it's inside a frame this action cannot address");
    case 'ambiguous':
      return fail(
        `found ${pick.count} file fields on this page and cannot tell which one to use — attach it by hand`,
      );
  }
  const upload = pick.upload;

  if (!cv) return fail('no tailored CV found for this job');

  const pdfBytes = await getCVPdfBytes(cv.id, token);
  const dataUrl = pdfDataUrl(pdfBytes);
  const filename = `freehire-tailored-cv/${cv.job_slug}.pdf`;

  let filePath: string;
  try {
    filePath = await resolveDownloadedPath(realDownloadsAPI, dataUrl, filename, DOWNLOAD_TIMEOUT_MS);
  } catch (err) {
    return fail(errorMessage(err));
  }

  const debuggee = { tabId };
  try {
    await browser.debugger.attach(debuggee, '1.3');
  } catch (err) {
    return fail(classifyAttachError(errorMessage(err)));
  }
  try {
    const evalResult = (await browser.debugger.sendCommand(debuggee, 'Runtime.evaluate', {
      expression: uploadInputExpression(upload.form),
    })) as { result?: { objectId?: string } } | undefined;
    const objectId = evalResult?.result?.objectId;
    if (!objectId) return fail('could not find the upload field on the page any more');

    await browser.debugger.sendCommand(debuggee, 'DOM.setFileInputFiles', {
      objectId,
      files: [filePath],
    });
    return { kind: 'ATTACH_TAILORED_CV_RESULT', ok: true };
  } catch (err) {
    return fail(errorMessage(err));
  } finally {
    await browser.debugger.detach(debuggee).catch(() => {
      // Already detached (e.g. the tab closed mid-call) — nothing left to clean up.
    });
  }
}
