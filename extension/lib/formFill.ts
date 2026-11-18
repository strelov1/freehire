import { browser } from 'wxt/browser';
import { track } from './analytics';
import type { AnswerOffer } from './answerOffer';
import { getAutofillProfile, runAgentAutofill, saveBankAnswer } from './freehire';
import { profileToValues } from './form';
import { looksLikeApplication, planLabelFills, scopeToApplication } from './fillPlan';
import { walkFills, walkReported, type WalkHooks } from './formWalk';
import { PlanRefused } from './planRefused';
import type { RuntimeMessage } from './protocol';

/** What a fill tells the side panel, and asks of it. */
export interface FillPanel {
  /** A line for the panel's notices. */
  notice(text: string): void;
  /** Offer "Fill it anyway" with what it would run, or withdraw the offer (null). */
  offerOverride(run: (() => Promise<void>) | null): void;
  /** What the walk reports back. */
  walk: WalkHooks;
  /** Read the page's form again, so the checklist follows what was filled. */
  refreshPlan(): Promise<void>;
  /** A question whose answer freehire now holds, so it is not offered for saving. */
  known(label: string): void;
  /** The plan refused the agent: the panel shows its upgrade offer. */
  refused(err: PlanRefused): void;
}

/**
 * Names the first few labels and counts the rest. A real ATS form leaves
 * dozens of fields untouched (Greenhouse alone contributes a checkbox per
 * country), and a notice that lists them all is one the user cannot read.
 */
function nameSome(labels: string[], shown = 5): string {
  const trimmed = labels.map((l) => l.trim().replace(/\s+/g, ' ')).filter(Boolean);
  if (trimmed.length <= shown) return trimmed.join(', ');
  return `${trimmed.slice(0, shown).join(', ')} and ${trimmed.length - shown} more`;
}

/**
 * Autofill, agent-first: freehire's agent reads the form through the wire, maps the
 * profile onto it, and fills what it can justify; the deterministic filler below stays as
 * the fallback. The agent's report is played back over the page so the user watches what
 * changed rather than finding it later.
 */
export async function agentAutofill(token: string, panel: FillPanel): Promise<void> {
  try {
    // Read the form first: the walk ticks questions off the plan, and a page
    // whose form arrived after the last read (an ATS step change, which fires
    // no page load) would otherwise be walked with nothing to tick.
    await panel.refreshPlan();
    const report = await runAgentAutofill(token);
    // The agent filled the page itself, server-side. Play its report back so
    // the user watches what changed rather than finding it later.
    if (report.filled.length > 0) panel.walk.wrote();
    // Every label the agent filled is known up front: a walk stopped part-way still
    // leaves those answers written, and they came from the profile, not the candidate.
    for (const label of report.filled) panel.known(label);
    await walkReported(report.filled, panel.walk);
    const filled = report.filled.length;
    track('autofill', { mode: 'agent', filled });
    panel.notice(
      filled > 0
        ? `✓ Autofilled ${filled} field${filled === 1 ? '' : 's'} — review before submitting.`
        : 'The agent found nothing on this form it could fill from your profile.',
    );
    if (report.deferred.length > 0) {
      panel.notice(`Not fillable yet (custom dropdowns): ${nameSome(report.deferred)}.`);
    }
  } catch (err) {
    if (err instanceof PlanRefused) {
      // The plan said no: a paywall with the upgrade link, not an error line.
      panel.refused(err);
      panel.notice('Using the basic filler instead.');
    } else {
      // The server's own sentence, not just the status: /me/autofill/run answers
      // 409 for three unrelated states, and only that sentence says which.
      panel.notice(
        `Agent autofill unavailable: ${err instanceof Error ? err.message : 'error'} — using the basic filler.`,
      );
    }
    track('autofill', { mode: 'basic' });
    await deterministicAutofill(token, panel);
  }
}

/**
 * The fallback filler, over the same frame-aware primitives the agent drives:
 * an apply form is routinely served from an ATS iframe, and a careers page
 * carrying any other iframe would otherwise be answered by whichever frame
 * replied first. Addressing questions by label rather than by position also
 * keeps the read and the write on the same question when a form re-renders
 * between them.
 */
async function deterministicAutofill(token: string, panel: FillPanel, force = false): Promise<void> {
  try {
    const formReply = (await browser.runtime.sendMessage({
      kind: 'GET_FRAMED_FORM',
    } satisfies RuntimeMessage)) as RuntimeMessage | undefined;
    if (formReply?.kind !== 'FRAMED_FORM' || formReply.fields.length === 0) {
      panel.notice('No form fields found on this page.');
      return;
    }

    // A careers page keeps the application behind an "Apply" button and shows a
    // job-alert signup meanwhile; filling that one silently is worse than
    // declining, so the user is told and can insist.
    if (!force && !looksLikeApplication(formReply.uploads)) {
      panel.notice(
        `This doesn't look like the application form — ${formReply.fields.length} field${
          formReply.fields.length === 1 ? '' : 's'
        } are showing and none of them takes a CV. Open the application on the page, then try again.`,
      );
      panel.offerOverride(() => deterministicAutofill(token, panel, true));
      return;
    }
    panel.offerOverride(null);

    // One form, not every question on the page: an application and a job-alert
    // signup each have their own "Email".
    const scoped = scopeToApplication(formReply.fields, formReply.uploads);
    const fills = planLabelFills(scoped, profileToValues(await getAutofillProfile(token)));
    if (fills.length === 0) {
      panel.notice('Nothing matched your profile on this form.');
      return;
    }
    const walk = await walkFills(fills, panel.walk);
    // The form has now accepted values, which is stronger evidence than
    // anything its markup says — so read it again and let the checklist appear.
    await panel.refreshPlan();
    const n = walk.done.length;
    // What was left unanswered is not listed here: the checklist above shows
    // exactly which questions those are, and naming thirty of them in a notice
    // is the wall of text this feature replaced.
    panel.notice(
      walk.stopped
        ? `Stopped after ${n} field${n === 1 ? '' : 's'} — what was filled stays.`
        : `✓ Autofilled ${n} field${n === 1 ? '' : 's'} — review before submitting.`,
    );
  } catch (err) {
    panel.notice(`Autofill failed: ${err instanceof Error ? err.message : 'error'}`);
  }
}

/**
 * Saves the answers the candidate chose into their answer bank, one at a time. One
 * refused answer (too long, no readable question) does not stop the rest; the
 * notice names what did not go through.
 *  `panel.saved` is told each question that reached the bank.
 */
export async function saveChosenAnswers(
  chosen: AnswerOffer[],
  token: string,
  panel: { notice(text: string): void; saved(question: string): void },
): Promise<void> {
  const failed: string[] = [];
  for (const offer of chosen) {
    try {
      await saveBankAnswer(offer.question, offer.answer, token);
      // Saved answers leave the offer: the next read must not ask for them again.
      panel.saved(offer.question);
    } catch {
      failed.push(offer.question);
    }
  }
  const saved = chosen.length - failed.length;
  track('answers_saved', { saved, failed: failed.length });
  if (saved > 0) {
    panel.notice(`✓ Saved ${saved} answer${saved === 1 ? '' : 's'} — freehire will fill them on the next form.`);
  }
  if (failed.length > 0) panel.notice(`Could not save: ${nameSome(failed)}.`);
}
