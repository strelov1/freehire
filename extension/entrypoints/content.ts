import { extractSnapshot } from '../lib/scraper';
import { extractForm, extractUploads, fillByLabel } from '../lib/form';
import { runStep } from '../lib/combobox';
import type { RuntimeMessage } from '../lib/protocol';

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
          return Promise.resolve<RuntimeMessage>({
            kind: 'FILL_OUTCOMES',
            outcomes: fillByLabel(document, message.fills),
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
  },
});
