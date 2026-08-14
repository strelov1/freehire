import {
  emptySnapshot,
  type RuntimeMessage,
  type FramedField,
  type FramedUpload,
  type LabelFill,
  type FillOutcome,
  type ComboboxStep,
  type ComboboxReply,
} from '../lib/protocol';
import { mergeComboboxReplies, mergeFrameOutcomes } from '../lib/tools/executor';
import { fillsForFrame } from '../lib/form';

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
        return fillAcrossFrames(message.fills);
      case 'COMBOBOX_STEP':
        return comboboxAcrossFrames(message.step);
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
async function fillAcrossFrames(fills: LabelFill[]): Promise<RuntimeMessage> {
  const perFrame: FillOutcome[][] = [];
  await eachFrame(
    (frame) => ({ kind: 'FILL_BY_LABEL', fills: fillsForFrame(fills, frame) }),
    (reply) => {
      if (reply?.kind === 'FILL_OUTCOMES') perFrame.push(reply.outcomes);
    },
  );
  return { kind: 'FILL_OUTCOMES', outcomes: mergeFrameOutcomes(perFrame) };
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
): Promise<void> {
  const tabId = await activeTabId();
  if (tabId == null) return;
  const forFrame = typeof message === 'function' ? message : () => message;
  await Promise.all(
    (await frameIds(tabId)).map(async (frameId) => {
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
