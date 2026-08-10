/**
 * The `PageBridge` backed by the real browser: it asks the background relay,
 * which fans the request out across the active tab's frames. Thin glue — the
 * dispatch and merge logic it feeds lives in `executor.ts`, where it is tested.
 */

import { browser } from 'wxt/browser';
import type { ComboboxStep, LabelFill, RuntimeMessage } from '../protocol';
import type { FormObservation, PageBridge } from './executor';
import { isReadablePageUrl, NOT_A_WEB_PAGE } from './readable';

async function ask(message: RuntimeMessage): Promise<RuntimeMessage | undefined> {
  return (await browser.runtime.sendMessage(message)) as RuntimeMessage | undefined;
}

export const activeTabPage: PageBridge = {
  async readPage() {
    // Decided from the url, before the page is read — the point is to not read it.
    const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
    if (!isReadablePageUrl(tab?.url)) throw new Error(NOT_A_WEB_PAGE);

    const reply = await ask({ kind: 'GET_PAGE_SNAPSHOT' });
    // The background answers "no active tab" with an empty snapshot rather than a
    // failure, which suits the match card (it just shows nothing). Here it would
    // hand the agent a blank page to narrate about, so an empty url is an error.
    if (reply?.kind !== 'PAGE_SNAPSHOT' || !reply.snapshot.url) {
      throw new Error('no page to read: the active tab could not be reached');
    }
    return reply.snapshot;
  },

  async readForm(): Promise<FormObservation> {
    const reply = await ask({ kind: 'GET_FRAMED_FORM' });
    if (reply?.kind !== 'FRAMED_FORM') throw new Error('could not read the page');
    return { fields: reply.fields, uploads: reply.uploads };
  },

  async fillSimple(fills: LabelFill[]) {
    const reply = await ask({ kind: 'FILL_BY_LABEL', fills });
    if (reply?.kind !== 'FILL_OUTCOMES') throw new Error('could not reach the page to fill it');
    return reply.outcomes;
  },

  async combobox(step: ComboboxStep) {
    const reply = await ask({ kind: 'COMBOBOX_STEP', step });
    if (reply?.kind !== 'COMBOBOX_REPLY') throw new Error('could not reach the page to drive the widget');
    return reply.reply;
  },
};
