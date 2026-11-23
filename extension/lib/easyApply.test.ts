import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  easyApplyAction,
  easyApplyDialog,
  easyApplyOnResumeStep,
  easyApplySent,
  linkedInPostingClosed,
} from './easyApply';
import { extractForm, extractUploads, fillByLabel, numericAnswer } from './form';
import { buildPlan } from './applyPlan';
import { looksLikeApplication } from './fillPlan';
import { easyApplyUploadButtonExpression } from './tools/attachCv';
import { must } from './test-utils';

/** A captured Easy Apply step, with LinkedIn's search box outside the dialog as on the live page. */
function load(step: string) {
  const html = readFileSync(resolve(process.cwd(), 'lib/fixtures', `linkedin-easy-apply-${step}.html`), 'utf8');
  document.documentElement.innerHTML = html;
  document.body.insertAdjacentHTML('afterbegin', '<input type="text" aria-label="Search">');
}

describe('LinkedIn Easy Apply', () => {
  it('counts as an application although it offers no file field', () => {
    load('step1-contact');
    const uploads = extractUploads(document);
    expect(uploads).toEqual([]);
    expect(looksLikeApplication(uploads, easyApplyDialog(document) !== null)).toBe(true);
  });

  it('reads only the dialog, not the page around it', () => {
    load('step1-contact');
    expect(extractForm(document).map((f) => f.label)).toEqual(['Email*', 'Phone country code*', 'Teléfono móvil*']);
  });

  it('leaves the stored-resume picker out of the questions', () => {
    load('step2-resume');
    expect(extractForm(document)).toEqual([]);
  });

  it('reads an employer question as required', () => {
    load('step3-questions');
    expect(extractForm(document).map((f) => [f.label, f.required])).toEqual([
      ['Cuál es tu expectativa salarial', true],
    ]);
  });

  it('is not seen once the dialog is closed', () => {
    load('step1-contact');
    must(document.querySelector('dialog'), 'dialog').removeAttribute('open');
    expect(easyApplyDialog(document)).toBeNull();
  });

  it.each([
    ['step1-contact', 'Next', 'advance'],
    ['step3-questions', 'Review', 'advance'],
  ])('finds the %s footer button by position', (step, text, kind) => {
    load(step);
    const action = must(easyApplyAction(must(easyApplyDialog(document), 'dialog')), 'action');
    expect([action.button.textContent?.trim(), action.kind]).toEqual([text, kind]);
  });

  it.each([
    ['Submit application', 'submit'],
    ['Weiter', 'unknown'],
  ])('classifies a footer button reading %s as %s', (text, kind) => {
    load('step3-questions');
    const dialog = must(easyApplyDialog(document), 'dialog');
    const buttons = dialog.querySelectorAll('footer button');
    must(buttons[buttons.length - 1], 'button').textContent = text;
    expect(easyApplyAction(dialog)?.kind).toBe(kind);
  });

  it('reads a rejected answer as not answered', () => {
    load('step3-questions');
    const [field] = extractForm(document);
    expect(field?.invalid).toBe('Invalid input');
    const plan = buildPlan([{ ...must(field, 'field'), value: '5000', frame: 0 }]);
    expect(plan.items[0]?.answered).toBe(false);
  });

  it('reads an answer it has not rejected as answered', () => {
    load('step3-questions');
    must(document.querySelector('[data-testid="text-input-helper-text"] > p'), 'error').remove();
    const [field] = extractForm(document);
    expect(field?.invalid).toBeUndefined();
  });

  it('fills the salary question with a bare number', () => {
    load('step3-questions');
    const [outcome] = fillByLabel(document, [
      { label: 'Cuál es tu expectativa salarial', value: '5000 USD/month', form: -1 },
    ]);
    expect(outcome?.status).toBe('filled');
    expect((must(document.getElementById('_r_2s_'), 'input') as HTMLInputElement).value).toBe('5000');
  });
});

describe('numericAnswer', () => {
  it.each([
    ['5000 USD/month', '5000'],
    ['€60,000 per year', '60000'],
    ['60.000 EUR', '60000'],
    ['60k', '60000'],
    ['4.5k', '4500'],
    ['3,5', '4'],
    ['50000-60000', '50000'],
    ['negotiable', null],
  ])('%s → %s', (value, expected) => {
    expect(numericAnswer(value)).toBe(expected);
  });
});

describe('a salary question outside Easy Apply', () => {
  it('keeps the profile text in a free-text field', () => {
    document.documentElement.innerHTML = '<body><label for="s">Desired salary</label><input id="s" type="text"></body>';
    fillByLabel(document, [{ label: 'Desired salary', value: '5000 USD/month', form: -1 }]);
    expect((must(document.getElementById('s'), 's') as HTMLInputElement).value).toBe('5000 USD/month');
  });

  it('writes a number into a number input', () => {
    document.documentElement.innerHTML =
      '<body><label for="s">Desired salary</label><input id="s" type="number"></body>';
    fillByLabel(document, [{ label: 'Desired salary', value: '5000 USD/month', form: -1 }]);
    expect((must(document.getElementById('s'), 's') as HTMLInputElement).value).toBe('5000');
  });
});

describe('the Easy Apply upload button expression', () => {
  it('points at "Upload resume" on the resume step', () => {
    load('step2-resume');
    Element.prototype.scrollIntoView ??= () => {};
    const point = (0, eval)(easyApplyUploadButtonExpression()) as { x: number; y: number } | null;
    expect(point).not.toBeNull();
  });

  it('finds nothing on a step without the resume picker', () => {
    load('step3-questions');
    expect((0, eval)(easyApplyUploadButtonExpression())).toBeNull();
  });
});

// Minimal fixture after the job view seen live on a closed posting (2026-11-22): the
// top card's red notice and the apply button rendered disabled. Not a captured page.
describe('linkedInPostingClosed', () => {
  const topCard = (button: string, notice = '') =>
    (document.body.innerHTML = `<main><div class="job-details-jobs-unified-top-card__container--two-pane">
      <h1>Senior Engineer</h1>${notice}
      <div class="jobs-apply-button--top-card">${button}</div></div></main>`);

  it('reads a disabled apply button as closed', () => {
    topCard(
      '<button id="jobs-apply-button-id" class="jobs-apply-button" aria-label="Easy Apply to this job" disabled>Easy Apply</button>',
    );
    expect(linkedInPostingClosed(document)).toBe(true);
  });

  it('falls back to the notice text when the button is gone', () => {
    topCard('', '<div class="artdeco-inline-feedback--error"><span>No longer accepting applications</span></div>');
    expect(linkedInPostingClosed(document)).toBe(true);
  });

  it('leaves an open posting open', () => {
    topCard(
      '<button id="jobs-apply-button-id" class="jobs-apply-button" aria-label="Easy Apply to this job">Easy Apply</button>',
    );
    expect(linkedInPostingClosed(document)).toBe(false);
  });
});

describe('easyApplyOnResumeStep', () => {
  it.each([
    ['step1-contact', false],
    ['step2-resume', true],
    ['step3-questions', false],
  ])('%s -> %s', (step, expected) => {
    load(step);
    expect(easyApplyOnResumeStep(document)).toBe(expected);
  });
});

describe('easyApplySent', () => {
  it('reads the confirmation inside an open dialog, not the page behind it', () => {
    document.body.innerHTML = '<p>Your application was sent in the posting text</p>';
    expect(easyApplySent(document)).toBe(false);
    document.body.innerHTML = '<dialog open><h2>Your application was sent to Acme</h2></dialog>';
    expect(easyApplySent(document)).toBe(true);
    load('step1-contact');
    expect(easyApplySent(document)).toBe(false);
  });
});
