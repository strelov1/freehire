import { describe, it, expect } from 'vitest';
import { shouldRequest, forDisplay, MAX_DISPLAY_LEN } from './followups';

describe('shouldRequest', () => {
  it('asks after a turn that ended normally with an answer', () => {
    expect(shouldRequest({ type: 'result', stop_reason: 'end_turn' }, 'here are three roles')).toBe(
      true,
    );
  });

  it('does not ask after a turn that produced no words', () => {
    // Nothing was said, so there is nothing to follow up on — and the call would be
    // spent finding that out.
    expect(shouldRequest({ type: 'result', stop_reason: 'end_turn' }, '')).toBe(false);
    expect(shouldRequest({ type: 'result', stop_reason: 'end_turn' }, '   ')).toBe(false);
  });

  it('does not ask after a turn the caller has to resolve', () => {
    // An error, a cancellation and the step ceiling all leave the conversation in a
    // state the caller must act on. Suggesting what to ask next there reads as if
    // nothing had gone wrong.
    for (const stop_reason of ['error', 'cancelled', 'max_steps'] as const) {
      expect(shouldRequest({ type: 'result', stop_reason }, 'some text')).toBe(false);
    }
  });

  it('does not ask when the turn ended in error despite its stop reason', () => {
    expect(shouldRequest({ type: 'result', stop_reason: 'end_turn', is_error: true }, 'text')).toBe(
      false,
    );
  });

  it('does not ask for a stop reason it does not recognise', () => {
    // The backend owns this vocabulary. An unknown value is a reason to stay quiet,
    // not a reason to guess.
    expect(shouldRequest({ type: 'result', stop_reason: 'something_new' }, 'text')).toBe(false);
    expect(shouldRequest({ type: 'result' }, 'text')).toBe(false);
  });
});

describe('forDisplay', () => {
  it('leaves an ordinary suggestion alone', () => {
    expect(forDisplay('compare the first two?')).toBe('compare the first two?');
  });

  it('collapses newlines, which would otherwise break the row', () => {
    expect(forDisplay('compare\nthe first\ttwo?')).toBe('compare the first two?');
  });

  it('truncates past the display limit', () => {
    const long = 'a'.repeat(MAX_DISPLAY_LEN + 20);
    const shown = forDisplay(long);
    expect(shown.length).toBeLessThanOrEqual(MAX_DISPLAY_LEN + 1);
    expect(shown.endsWith('…')).toBe(true);
  });

  it('trims surrounding whitespace', () => {
    expect(forDisplay('  ask this?  ')).toBe('ask this?');
  });
});
