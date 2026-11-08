import { describe, it, expect, beforeEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { extractForm, extractUploads, fillByLabel, looksLikeApplication, matchFieldKey } from './form';
import type { LabelFill } from './protocol';
import { must } from './test-utils';

/** Real ATS markup with personal values blanked; see the comment in each file. */
function load(name: string) {
  const html = readFileSync(resolve(process.cwd(), 'lib/fixtures', name), 'utf8');
  document.documentElement.innerHTML = html;
}

function input(id: string) {
  return must(document.getElementById(id), id) as HTMLInputElement;
}

describe.each([
  {
    name: 'Personio',
    file: 'personio-de.html',
    // Personio appends its screen-reader hint to required labels.
    labels: [
      'Vorname',
      'Nachname',
      'E-Mail* (erforderlich)',
      'Telefon',
      'Verfügbar ab* (erforderlich)',
      'Gehaltsvorstellung* (erforderlich)',
      'Geschlecht* (erforderlich)',
    ],
    matched: {
      Vorname: 'firstName',
      Nachname: 'lastName',
      'E-Mail* (erforderlich)': 'email',
      Telefon: 'phone',
      'Gehaltsvorstellung* (erforderlich)': 'desiredSalary',
    } as Record<string, string>,
    ids: {
      Vorname: 'field-first_name',
      Nachname: 'field-last_name',
      'E-Mail* (erforderlich)': 'field-email',
      Telefon: 'field-phone',
      'Gehaltsvorstellung* (erforderlich)': 'field-salary_expectations',
    } as Record<string, string>,
    untouched: ['field-available_from', 'field-gender'],
  },
  {
    name: 'softgarden',
    file: 'softgarden-de.html',
    labels: [
      'Anrede',
      'Vorname *',
      'Nachname *',
      'E-Mail *',
      'Telefon *',
      'Wie ist Ihre Kündigungsfrist? *',
      'Bitte geben Sie Ihren Gehaltswunsch an *',
    ],
    matched: {
      'Vorname *': 'firstName',
      'Nachname *': 'lastName',
      'E-Mail *': 'email',
      'Telefon *': 'phone',
    } as Record<string, string>,
    ids: {
      'Vorname *': 'id3',
      'Nachname *': 'id4',
      'E-Mail *': 'id6',
      'Telefon *': 'id9',
    } as Record<string, string>,
    untouched: ['id1', 'idb', 'id13', 'id14'],
  },
])('$name fixture', ({ file, labels, matched, ids, untouched }) => {
  beforeEach(() => load(file));

  it('is recognised as an application form', () => {
    expect(looksLikeApplication(extractUploads(document))).toBe(true);
  });

  it('observes the German labels as questions', () => {
    const seen = extractForm(document).map((f) => f.label);
    for (const label of labels) expect(seen).toContain(label);
  });

  it('maps German labels to profile keys, and no other observed label to any key', () => {
    for (const { label } of extractForm(document)) {
      expect(matchFieldKey(label), label).toBe(matched[label] ?? null);
    }
  });

  it('fills matched fields through fillByLabel and leaves the rest alone', () => {
    const before = Object.fromEntries(untouched.map((id) => [id, (input(id) as HTMLInputElement).value]));
    const fills: LabelFill[] = Object.keys(matched).map((label) => ({
      label,
      value: `v-${matched[label]}`,
    }));
    const outcomes = fillByLabel(document, fills);

    expect(outcomes).toEqual(fills.map(({ label }) => ({ label, status: 'filled' })));
    for (const label of Object.keys(matched)) expect(input(must(ids[label], label)).value).toBe(`v-${matched[label]}`);
    for (const id of untouched) expect(input(id).value).toBe(before[id]);
    for (const upload of Array.from(document.querySelectorAll<HTMLInputElement>('input[type=file]'))) {
      expect(upload.files?.length ?? 0).toBe(0);
    }
  });
});

describe('softgarden question-shaped labels', () => {
  beforeEach(() => load('softgarden-de.html'));

  // The synonym has to open the label, so a full question never matches. These are
  // left to the agent path; this test documents the boundary rather than hiding it.
  it('stay unmatched by the deterministic filler', () => {
    expect(matchFieldKey('Wie ist Ihre Kündigungsfrist? *')).toBeNull();
    expect(matchFieldKey('Bitte geben Sie Ihren Gehaltswunsch an *')).toBeNull();
  });
});
