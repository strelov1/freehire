import { extractSnapshot } from '../lib/scraper';
import { extractForm, extractUploads, fillByLabel, revealField } from '../lib/form';
import { runStep } from '../lib/combobox';
import { debounce } from '../lib/debounce';
import type { RuntimeMessage } from '../lib/protocol';

/** How long the page waits for typing to stop before telling the panel its form
 *  changed. One re-read when the user pauses, not one per keystroke. */
const FORM_CHANGE_QUIET_MS = 400;

/** Whether a node is, or contains, something the panel would ask about. Keeps the
 *  observer below from waking the panel for every unrelated re-render — an ATS
 *  page swaps nodes constantly. */
function holdsControl(node: Node): boolean {
  if (!(node instanceof Element)) return false;
  return node.matches('input, select, textarea') || node.querySelector('input, select, textarea') !== null;
}

/**
 * Injected into every page, and into every frame of it — apply forms are
 * routinely served from an ATS iframe, so a top-document-only script would see
 * an empty page. The extension's eyes + hands: it reads the page (snapshot,
 * form) and writes fills back. Owns no state; the background relay addresses
 * each frame individually and tags what comes back.
 */
export default defineContentScript({
  matches: ['<all_urls>'],
  allFrames: true,
  main() {
    browser.runtime.onMessage.addListener((message: RuntimeMessage) => {
      switch (message.kind) {
        case 'GET_PAGE_SNAPSHOT':
          return Promise.resolve<RuntimeMessage>({
            kind: 'PAGE_SNAPSHOT',
            snapshot: extractSnapshot(document),
          });
        case 'GET_FRAMED_FORM':
          // The frame tag is stamped by the background relay, which is the only
          // side that knows this frame's id. The uploads travel alongside the
          // questions: they are not fillable, they say whether this is an
          // application at all.
          return Promise.resolve<RuntimeMessage>({
            kind: 'FORM',
            fields: extractForm(document),
            uploads: extractUploads(document),
          });
        case 'FILL_BY_LABEL':
          // A revealed fill scrolls to each question and outlines it as the value
          // lands — the panel's walk, watched. The reveal runs first so the user
          // is looking at the control before it changes.
          if (message.reveal) {
            for (const fill of message.fills) {
              revealField(document, { label: fill.label, form: fill.form });
            }
          }
          return Promise.resolve<RuntimeMessage>({
            kind: 'FILL_OUTCOMES',
            outcomes: fillByLabel(document, message.fills),
          });
        case 'REVEAL_FIELD':
          return Promise.resolve<RuntimeMessage>({
            kind: 'REVEAL_RESULT',
            found: revealField(document, message.request),
          });
        case 'COMBOBOX_STEP':
          // The only asynchronous handler here: a widget re-renders when its
          // framework decides to, so the step awaits the result.
          return runStep(document, message.step).then(
            (reply): RuntimeMessage => ({ kind: 'COMBOBOX_REPLY', reply }),
          );
        default:
          return undefined;
      }
    });

    // Answers the user types themselves must move the panel's counter, or it
    // reports a form as less finished than it is. One delegated listener rather
    // than one per control: an ATS form re-renders constantly, and per-control
    // listeners would have to be re-attached on every render (and leak the ones
    // they replaced). The panel is told only that something changed — it re-reads
    // the form, which is the one account that cannot drift.
    const notifyPanel = () => {
      void browser.runtime.sendMessage({ kind: 'FORM_CHANGED' } satisfies RuntimeMessage).catch(() => {
        // No panel listening (it is closed, or this frame outlived it). Nothing
        // to do: the next open re-reads the form anyway.
      });
    };
    const announce = debounce(notifyPanel, FORM_CHANGE_QUIET_MS);
    document.addEventListener('input', announce, true);
    document.addEventListener('change', announce, true);

    // A form the page renders later — the next step of an ATS application, an
    // "Apply" button expanding one in place — arrives with no page load and no
    // typing, so neither the panel's tab listeners nor the two above would ever
    // notice it.
    //
    // Only a change in HOW MANY controls the page holds is announced. A React ATS
    // form re-renders its inputs constantly, and announcing every re-render kept
    // the panel re-reading the form without pause — which is what starved its
    // reads and left the checklist permanently absent. Typing is already covered
    // by the two listeners above, where the count does not change.
    let controlCount = document.querySelectorAll('input, select, textarea').length;
    const announceIfCountChanged = debounce(() => {
      const count = document.querySelectorAll('input, select, textarea').length;
      if (count === controlCount) return;
      controlCount = count;
      notifyPanel();
    }, FORM_CHANGE_QUIET_MS);
    new MutationObserver((records) => {
      for (const record of records) {
        if ([...record.addedNodes, ...record.removedNodes].some(holdsControl)) {
          announceIfCountChanged();
          return;
        }
      }
    }).observe(document.documentElement, { childList: true, subtree: true });
  },
});
