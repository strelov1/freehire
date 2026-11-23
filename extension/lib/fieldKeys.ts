// Matching a form field's label to the profile key it asks for, and the label
// normalisation both the matcher and the form executor (form.ts) compare by.

import { countryLabel } from './labels';

/**
 * Canonical profile keys and the label synonyms that map to them. German ATS forms
 * (Personio, softgarden, SuccessFactors) use the German synonyms. The English
 * 'email' / 'e-mail' prefixes already open "E-Mail-Adresse" and "Email-Adresse"
 * (a hyphen ends the word), so email needs no German entry.
 */
const FIELD_SYNONYMS: Record<string, string[]> = {
  fullName: ['full name', 'your name', 'candidate name', 'applicant name'],
  firstName: ['first name', 'given name', 'legal first name', 'preferred first name', 'vorname'],
  lastName: ['last name', 'family name', 'surname', 'legal last name', 'nachname', 'familienname'],
  email: ['email', 'e-mail', 'e-mail address', 'email address'],
  phone: [
    'phone',
    'mobile',
    'telephone',
    'contact number',
    'cell',
    'telefon',
    'telefonnummer',
    'mobilnummer',
    'handynummer',
    'rufnummer',
    'mobil',
  ],
  city: ['city', 'town', 'current location', 'location (city)', 'ort', 'wohnort', 'stadt'],
  state: ['state', 'province', 'region', 'bundesland'],
  country: ['country', 'land'],
  postalCode: ['postal code', 'zip', 'zip code', 'pincode', 'pin code', 'plz', 'postleitzahl'],
  linkedin: ['linkedin'],
  github: ['github'],
  portfolio: ['portfolio', 'website', 'personal site', 'webseite', 'homepage'],
  // The candidate's own screening answers (internal/screeninganswers), not a
  // boolean "are you authorized to work" question — that asks something the
  // profile does not carry an answer to, so it stays unmatched rather than
  // being fed a country list where a Yes/No answer belongs.
  authorizedCountries: [
    'which countries are you authorized to work in',
    'in which countries are you authorized to work',
    'in which countries are you legally authorized to work',
    'countries you are authorized to work in',
    'list the countries you are authorized to work in',
  ],
  visaSponsorshipNeeded: [
    'will you now or in the future require sponsorship',
    'do you now or will you in the future require sponsorship',
    'will you require sponsorship',
    'do you require visa sponsorship',
    'do you need visa sponsorship',
    'will you need visa sponsorship',
  ],
  desiredSalary: [
    'desired salary',
    'desired compensation',
    'expected salary',
    'salary expectation',
    'salary expectations',
    'what are your salary expectations',
    'compensation expectations',
    'what is your desired salary',
    'what is your expected salary',
    'what is your salary expectation',
    // LinkedIn Easy Apply asks in the employer's language.
    'cuál es tu expectativa salarial',
    'cual es tu expectativa salarial',
    'expectativa salarial',
    'pretensión salarial',
    'pretension salarial',
    'aspiración salarial',
    'qual é a sua pretensão salarial',
    'pretensão salarial',
    'gehaltsvorstellung',
    'gehaltsvorstellungen',
    'gehaltswunsch',
    'gewünschtes gehalt',
    'ihre gehaltsvorstellung',
  ],
  noticePeriod: [
    'notice period',
    'current notice period',
    'what is your notice period',
    'how much notice',
    'kündigungsfrist',
    'ihre kündigungsfrist',
  ],
  willingToRelocate: [
    'are you willing to relocate',
    'would you be willing to relocate',
    'willing to relocate',
    'are you open to relocating',
    'open to relocation',
    'umzugsbereitschaft',
    'sind sie umzugsbereit',
    'sind sie bereit umzuziehen',
  ],
  age18OrOlder: [
    'are you at least 18 years',
    'are you 18 years of age or older',
    'are you over the age of 18',
    'are you 18 or older',
    'will you be 18 years of age',
  ],
};

/**
 * The profile's authorized-countries field arrives as comma-joined ISO codes ("US, DE" —
 * screeninganswers.AutofillFields' wire format), but the question it answers is almost
 * always a checkbox group whose options are full country names ("United States"), matched
 * option-by-option against this value by `chosenOptions` below. Left as codes, the value
 * would never match any option and the group would silently stay unchecked.
 */
export function formatAuthorizedCountries(codes: string): string {
  if (!codes) return '';
  return codes
    .split(',')
    .map((c) => c.trim())
    .filter(Boolean)
    .map(countryLabel)
    .join(', ');
}

/**
 * Comparison form of a label: case-, whitespace- and required-marker-insensitive,
 * so "First Name *" and "first name" address the same control.
 */
export function normalizeLabel(label: string): string {
  return label.toLowerCase().replace(/\*/g, '').replace(/\s+/g, ' ').trim();
}

/**
 * Maps a control's label to a canonical profile key, or null when unknown.
 *
 * A synonym normally has to *open* the label, not merely appear in it. A real form asks
 * "Are you authorized to lawfully work for Roku in the country to which you are
 * applying?" — a substring match hands that Yes/No question the `country` key,
 * and the profile's country goes into a radio group. Anchoring at the front
 * still reads every label the live ATS forms produce, decorations and all
 * ("First Name (required) e85441b6"), because those trail the question rather
 * than lead it.
 *
 * The exception is a long synonym: a question with a preamble ("Para contratação na
 * modalidade Contractor ... qual a sua pretensão salarial?") is still asking it, and a phrase
 * of this length does not turn up inside an unrelated question. Openers win over these.
 */
export function matchFieldKey(label: string): string | null {
  const normalized = normalizeLabel(label);
  if (!normalized) return null;
  const entries = Object.entries(FIELD_SYNONYMS);
  for (const [key, synonyms] of entries) {
    if (synonyms.some((s) => opensWith(normalized, s))) return key;
  }
  for (const [key, synonyms] of entries) {
    if (synonyms.some((s) => s.length >= ANYWHERE_MIN_LEN && containsWord(normalized, s))) return key;
  }
  return null;
}

/** The shortest synonym trusted anywhere in a label rather than only at its start. */
const ANYWHERE_MIN_LEN = 15;

/** True when the label opens with this synonym and ends it at a word boundary. */
function opensWith(label: string, synonym: string): boolean {
  if (!label.startsWith(synonym)) return false;
  // "Citywide preference" is not a `city` question. äöüß count as letters too, so
  // German "Ort" does not claim "Ortsteil" nor "Land" claim "Landkreis".
  return !/[a-z0-9äöüß]/.test(label.charAt(synonym.length));
}

/** True when the synonym occurs in the label between word boundaries. */
function containsWord(label: string, synonym: string): boolean {
  for (let at = label.indexOf(synonym); at >= 0; at = label.indexOf(synonym, at + 1)) {
    if (!/[a-z0-9äöüß]/.test(label.charAt(at - 1)) && !/[a-z0-9äöüß]/.test(label.charAt(at + synonym.length)))
      return true;
  }
  return false;
}
