import { describe, expect, it } from 'vitest';
import { referralPeopleUrl } from './referralPeople';

describe('referralPeopleUrl', () => {
  it('uses the stored company people tab and drops tracking parameters', () => {
    expect(referralPeopleUrl('Acme', 'https://www.linkedin.com/company/acme/?trk=public')).toBe(
      'https://www.linkedin.com/company/acme/people/'
    );
  });

  it('accepts an existing school people tab', () => {
    expect(referralPeopleUrl('Acme University', 'http://linkedin.com/school/acme/people/')).toBe(
      'https://www.linkedin.com/school/acme/people/'
    );
  });

  it('falls back to a company search when the stored page is absent or invalid', () => {
    const search = 'https://www.linkedin.com/search/results/people/?keywords=Acme+%26+Co';
    expect(referralPeopleUrl(' Acme & Co ')).toBe(search);
    expect(referralPeopleUrl('Acme & Co', 'https://linkedin.com.evil.test/company/acme')).toBe(search);
    expect(referralPeopleUrl('Acme & Co', 'javascript:alert(1)')).toBe(search);
  });

  it('omits the action when the company name is unknown', () => {
    expect(referralPeopleUrl(' ', 'https://www.linkedin.com/company/acme/')).toBeNull();
  });
});
