import { describe, it, expect } from 'vitest';
import {
  DELETE_MS,
  HOLD_MS,
  PLACEHOLDER_ROLES,
  ROLE_PREFIX,
  TYPE_MS,
  ROLE_PLACEHOLDER,
  type TypewriterState,
  completeWord,
  typewriterStart,
  typedTail,
  typewriterStep,
} from './placeholderRoles';
import { CATEGORY_VALUES } from './generated/contracts';
import { categoryLabel } from './labels';

// The jobs search box's typed examples. The list is category KEYS, so these tests are
// partly about that guarantee holding at runtime too; the rest is the little state
// machine that types and deletes the trailing word, which has exactly the edge cases a
// hand-rolled typewriter always has — wrapping, an empty word, and being interrupted
// halfway through one.

describe('PLACEHOLDER_ROLES', () => {
  it('is a non-empty list with no duplicates', () => {
    expect(PLACEHOLDER_ROLES.length).toBeGreaterThan(0);
    expect(new Set(PLACEHOLDER_ROLES).size).toBe(PLACEHOLDER_ROLES.length);
  });

  it('names only categories the backend vocabulary carries', () => {
    for (const role of PLACEHOLDER_ROLES) {
      expect(CATEGORY_VALUES).toContain(role);
    }
  });

  it('resolves every key to a non-empty label', () => {
    for (const role of PLACEHOLDER_ROLES) {
      expect(categoryLabel(role).trim()).not.toBe('');
    }
  });
});

describe('ROLE_PLACEHOLDER', () => {
  // Pinned literally rather than re-derived through the same functions: mapping the same
  // inputs through the same code would restate the implementation and pass however the
  // wording changed. These are the words that ship.
  it('is a fixed prefix and the roles that follow it', () => {
    expect(ROLE_PLACEHOLDER).toEqual({
      prefix: 'Search jobs — e.g. ',
      roles: ['Backend', 'Frontend', 'DevOps', 'QA', 'Data Science', 'Product'],
    });
  });

  it('ends the prefix with a space, so the typed word is not glued to it', () => {
    expect(ROLE_PREFIX.endsWith(' ')).toBe(true);
  });
});

describe('typewriterStep', () => {
  const roles = ['ab', 'c'];
  const start = typewriterStart(roles);
  const run = (from: TypewriterState, times: number) => {
    const frames: { tail: string; delayMs: number }[] = [];
    let state = from;
    for (let i = 0; i < times; i++) {
      const { next, delayMs } = typewriterStep(state, roles);
      state = next;
      frames.push({ tail: typedTail(state, roles), delayMs });
    }
    return frames;
  };

  // The server renders this state, so it has to be a whole sentence: an empty tail would
  // ship "Search jobs - e.g. " to anyone reading before hydration or with JS off.
  it('starts with the first word already whole', () => {
    expect(typedTail(start, roles)).toBe('ab');
  });

  it('holds that finished word, then erases it a character at a time', () => {
    // The pause is the delay attached to the step that BEGINS the deletion, so the whole
    // word stays on screen for it.
    const frames = run(start, 3);
    expect(frames.map((f) => f.tail)).toEqual(['a', '', 'c']);
    expect(frames[0]?.delayMs).toBe(HOLD_MS);
    expect(frames[1]?.delayMs).toBe(DELETE_MS);
    expect(frames[2]?.delayMs).toBe(TYPE_MS);
  });

  it('types the next word one character at a time', () => {
    const frames = run({ role: 0, chars: 0, deleting: false }, 2);
    expect(frames[0]).toEqual({ tail: 'a', delayMs: TYPE_MS });
    expect(frames[1]).toEqual({ tail: 'ab', delayMs: TYPE_MS });
  });

  it('wraps from the last role back to the first', () => {
    const frames = run({ role: 1, chars: 0, deleting: true }, 1);
    expect(frames[0]?.tail).toBe('a');
  });

  // A category whose label resolved to nothing would otherwise drive `chars` negative and
  // render `slice(0, -1)` — the previous word minus a letter, which reads as a glitch
  // rather than as a missing label.
  it('skips a role with no label instead of counting past zero', () => {
    const withEmpty = ['', 'c'];
    const { next } = typewriterStep({ role: 0, chars: 0, deleting: false }, withEmpty);
    expect(next.role).toBe(1);
    expect(next.chars).toBeGreaterThanOrEqual(0);
  });
});

describe('completeWord', () => {
  // Freezing on "Devo" would read as a typo, so an interruption finishes the word first.
  it('fills in a half-typed word and stops deleting', () => {
    const state = completeWord({ role: 0, chars: 2, deleting: false }, ['Backend']);
    expect(state).toEqual({ role: 0, chars: 7, deleting: false });
    expect(typedTail(state, ['Backend'])).toBe('Backend');
  });

  it('fills in a word caught mid-deletion', () => {
    expect(typedTail(completeWord({ role: 0, chars: 1, deleting: true }, ['QA']), ['QA'])).toBe(
      'QA',
    );
  });

  it('leaves an already-complete word alone', () => {
    const done: TypewriterState = { role: 0, chars: 2, deleting: false };
    expect(completeWord(done, ['QA'])).toEqual(done);
  });
});
