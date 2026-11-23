import { describe, it, expect } from 'vitest';
import { matchFieldKey, formatAuthorizedCountries } from './fieldKeys';

describe('matchFieldKey', () => {
  it('maps common labels to canonical profile keys', () => {
    expect(matchFieldKey('First Name')).toBe('firstName');
    expect(matchFieldKey('Legal Last Name *')).toBe('lastName');
    expect(matchFieldKey('E-mail Address')).toBe('email');
    expect(matchFieldKey('LinkedIn URL')).toBe('linkedin');
  });

  it('maps German labels to canonical profile keys', () => {
    expect(matchFieldKey('Vorname')).toBe('firstName');
    expect(matchFieldKey('Vorname (erforderlich)')).toBe('firstName');
    expect(matchFieldKey('Nachname *')).toBe('lastName');
    expect(matchFieldKey('E-Mail-Adresse')).toBe('email');
    expect(matchFieldKey('Telefonnummer *')).toBe('phone');
    expect(matchFieldKey('PLZ')).toBe('postalCode');
    expect(matchFieldKey('Wohnort')).toBe('city');
    expect(matchFieldKey('Gehaltsvorstellung (brutto/Jahr)')).toBe('desiredSalary');
    expect(matchFieldKey('Kündigungsfrist')).toBe('noticePeriod');
  });

  it('does not let a German word claim a longer German word', () => {
    expect(matchFieldKey('Ortsteil')).toBeNull();
    expect(matchFieldKey('Landkreis')).toBeNull();
  });

  it('does not let a German word mid-sentence claim a key', () => {
    expect(matchFieldKey('In welchem Land haben Sie studiert?')).toBeNull();
  });

  it('returns null for an unknown label', () => {
    expect(matchFieldKey('Favourite colour')).toBeNull();
  });

  it('still reads a label an ATS has decorated', () => {
    // Greenhouse suffixes each label with a hash of its own.
    expect(matchFieldKey('First Name\n    (required)\n  \n  e85441b6')).toBe('firstName');
    expect(matchFieldKey('LinkedIn Profile 947f0cc4')).toBe('linkedin');
  });

  it('does not let a word buried in a question claim a profile key', () => {
    // Live on Roku. "country" appears mid-sentence in a Yes/No question; taking
    // it would put the user's country into a radio group.
    expect(
      matchFieldKey('Are you authorized to lawfully work for Roku in the country to which you are applying?'),
    ).toBeNull();
    expect(matchFieldKey('Which city office would your manager be based in?')).toBeNull();
  });

  it('does not match a longer word that merely starts the same way', () => {
    expect(matchFieldKey('Citywide preference')).toBeNull();
  });

  it('maps screening-answer questions to their canonical keys', () => {
    expect(matchFieldKey('Which countries are you authorized to work in?')).toBe('authorizedCountries');
    expect(matchFieldKey('Will you now or in the future require sponsorship to work in this country?')).toBe(
      'visaSponsorshipNeeded',
    );
    expect(matchFieldKey('Desired salary')).toBe('desiredSalary');
    expect(matchFieldKey('What is your notice period?')).toBe('noticePeriod');
    expect(matchFieldKey('Are you willing to relocate?')).toBe('willingToRelocate');
    expect(matchFieldKey('Are you at least 18 years of age?')).toBe('age18OrOlder');
  });

  it('leaves a plain work-authorization Yes/No question unmatched', () => {
    // That question has no boolean field in the profile — only a list of
    // authorized countries — so answering it would put the wrong shape of
    // value into a Yes/No control. See the Roku case above.
    expect(matchFieldKey('Are you authorized to work in this country?')).toBeNull();
  });
});

describe('formatAuthorizedCountries', () => {
  it('turns ISO codes into the country names a checkbox group carries as options', () => {
    expect(formatAuthorizedCountries('US, DE')).toBe('United States, Germany');
  });

  it('is empty for an empty value, rather than an empty-string entry', () => {
    expect(formatAuthorizedCountries('')).toBe('');
  });
});

describe('matchFieldKey with a preamble', () => {
  it('finds a long synonym anywhere in the label', () => {
    expect(
      matchFieldKey('Para contratação na modalidade Contractor (PJ), informe: Qual a sua pretensão salarial mensal?'),
    ).toBe('desiredSalary');
    expect(matchFieldKey('Please tell us: what is your notice period?')).toBe('noticePeriod');
  });

  it('does not read a currency question as the salary', () => {
    expect(matchFieldKey('In which currency are your salary expectations?')).toBeNull();
  });

  it('does not take a short synonym from the middle of a question', () => {
    expect(matchFieldKey('Which city would you prefer to work from?')).toBeNull();
    expect(matchFieldKey('Do you have a LinkedIn contact at our company?')).toBeNull();
  });

  it('keeps a long city synonym to the label start: a duration question is not the city', () => {
    expect(matchFieldKey('How long have you lived at your current location?')).toBeNull();
    expect(matchFieldKey('Current location')).toBe('city');
  });

  it('keeps word boundaries for long synonyms', () => {
    expect(matchFieldKey('Is the notice periodicity of our reviews fine?')).toBeNull();
  });
});
