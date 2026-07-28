import { describe, expect, it } from 'vitest';
import { entryFromQuery, historyModeFor } from './presets';

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

  it('matches the preset exactly', () => {
    expect(entry('preset=Profile')).toEqual({ preset: 'chat' });
  });
});

describe('historyModeFor', () => {
  it('replaces the bare address, which is a redirect and not a step', () => {
    expect(historyModeFor(undefined, 'a')).toBe('replace');
  });

  it('pushes a switch between two chats so Back returns to the one left', () => {
    expect(historyModeFor('a', 'b')).toBe('push');
  });

  it('does nothing when the address already names that chat', () => {
    // Opening a chat by its own URL calls back with the id already in the path; a
    // navigation here would be a history entry for standing still.
    expect(historyModeFor('a', 'a')).toBe('none');
  });
});
