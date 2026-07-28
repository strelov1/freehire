import { describe, expect, it } from 'vitest';
import { entryFromQuery } from './presets';

const entry = (query: string) => entryFromQuery(new URLSearchParams(query));

describe('entryFromQuery', () => {
  it('opens a plain chat and says nothing when no preset is asked for', () => {
    expect(entry('')).toEqual({ preset: 'chat' });
  });

  it('starts the experience interview with an opening message', () => {
    const { preset, kickoff } = entry('preset=profile');
    expect(preset).toBe('profile');
    expect(kickoff?.trim()).toBeTruthy();
  });

  it('leaves an explicit chat entry silent', () => {
    // The nav rail and a bookmarked chat both land here. Sending a message on the
    // caller's behalf there would put words in their mouth on every visit.
    expect(entry('preset=chat')).toEqual({ preset: 'chat' });
  });

  it('refuses a preset this surface cannot mint', () => {
    // `tailor` binds to a CV and a vacancy, so it is created by the tailoring page and
    // never by a URL. An unknown value is a typo, not an instruction.
    expect(entry('preset=tailor')).toEqual({ preset: 'chat' });
    expect(entry('preset=nonsense')).toEqual({ preset: 'chat' });
  });
});
