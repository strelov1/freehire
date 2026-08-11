import { describe, expect, it } from 'vitest';
import { entryFromQuery, historyModeFor, opensInRail, profileKickoff } from './presets';

const entry = (query: string) => entryFromQuery(new URLSearchParams(query));

/** First sentence of the stock profile kickoff — enough to assert it was kept. */
const PROFILE_KICKOFF_START = 'Walk through my experience with me';

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

  it('appends named atom ids to the profile kickoff', () => {
    const a = '11111111-1111-1111-1111-111111111111';
    const b = '22222222-2222-2222-2222-222222222222';
    const { kickoff } = entry(`preset=profile&atoms=${a},${b}`);
    expect(kickoff).toContain(PROFILE_KICKOFF_START);
    expect(kickoff).toContain(a);
    expect(kickoff).toContain(b);
    expect(kickoff).toMatch(/merge/i);
  });

  it('ignores malformed atoms and keeps the plain profile kickoff', () => {
    const { kickoff } = entry('preset=profile&atoms=not-a-uuid,also-bad');
    expect(kickoff?.trim()).toBeTruthy();
    expect(kickoff).not.toMatch(/Start with these achievements/);
  });

  it('does not invent a kickoff from atoms without the profile preset', () => {
    expect(entry('atoms=11111111-1111-1111-1111-111111111111')).toEqual({ preset: 'chat' });
  });
});

describe('profileKickoff', () => {
  const a = '11111111-1111-1111-1111-111111111111';
  const b = '22222222-2222-2222-2222-222222222222';

  it('falls back to the stock kickoff with no ids', () => {
    expect(profileKickoff([])).toContain(PROFILE_KICKOFF_START);
    expect(profileKickoff([])).not.toMatch(/Start with these achievements/);
  });

  it('names every id it was given', () => {
    const text = profileKickoff([a, b]);
    expect(text).toContain(a);
    expect(text).toContain(b);
  });

  it('drops anything that is not a server-minted id', () => {
    // The Experience panel passes ids straight from rendered rows and the URL entry
    // passes whatever was in the query. One filter covers both, so a query cannot
    // smuggle prose into the message we send as the candidate.
    expect(profileKickoff(['not-a-uuid', 'DROP TABLE'])).toBe(profileKickoff([]));
  });

  it('says each id once', () => {
    expect(profileKickoff([a, a])).toBe(profileKickoff([a]));
  });

  it('asks the agent to read the achievements before speaking about them', () => {
    // The agent used to be told to *search* these ids. Search retrieves by meaning and
    // cannot resolve one, so it reported achievements the candidate had just selected as
    // not existing, and answered about a different set.
    expect(profileKickoff([a, b])).toMatch(/read them first/i);
  });

  it('keeps our tool names out of a message recorded as the candidate’s', () => {
    // How to resolve an id belongs to the interviewer's prompt; this message says what to
    // do, not which tool to call.
    expect(profileKickoff([a, b])).not.toContain('experience_');
  });

  it('is what the URL entry sends, character for character', () => {
    // The two ways into the interview must ask the same thing. Comparing them against
    // each other rather than against a copied literal is what keeps that true when the
    // wording changes.
    expect(entry(`preset=profile&atoms=${a},${b}`).kickoff).toBe(profileKickoff([a, b]));
    expect(entry('preset=profile').kickoff).toBe(profileKickoff([]));
  });
});

describe('opensInRail', () => {
  it('opens every conversation the rail lists', () => {
    // These are the presets ListAssistantSessions returns. A browsing conversation
    // started in the extension's side panel is meant to be picked up here — refusing it
    // stranded anyone whose newest chat came from the extension, because boot() opens
    // the newest and the dead-link panel replaces the rail that would let them escape.
    expect(opensInRail('chat')).toBe(true);
    expect(opensInRail('profile')).toBe(true);
    expect(opensInRail('browse')).toBe(true);
  });

  it('refuses a conversation bound to an artifact', () => {
    // A tailoring chat belongs to the CV that owns it and is reached through the
    // tailoring workspace; opening one here shows a conversation the rail cannot list.
    expect(opensInRail('tailor')).toBe(false);
  });

  it('admits a preset it has not heard of', () => {
    // Deliberately open by default. The rail's contents are the backend's decision, and
    // a client whitelist that lags a new preset is exactly the bug this replaces.
    expect(opensInRail('something-new')).toBe(true);
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
