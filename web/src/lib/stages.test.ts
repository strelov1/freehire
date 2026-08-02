import { describe, expect, it } from 'vitest';

import { STAGE_GROUPS, STAGE_LABELS, STAGE_VALUES } from './generated/contracts';
import { groupedStages, offersDebrief } from './stages';

// The debrief reviews an interview that has already happened, so the offer follows the
// stage: it is noise on an application nobody has been interviewed for.
describe('offersDebrief', () => {
  it('offers the debrief once an interview has plausibly happened', () => {
    for (const stage of ['interview', 'offer', 'accepted']) {
      expect(offersDebrief(stage), stage).toBe(true);
    }
  });

  // A rejection that arrived after an interview is where a debrief is worth the most,
  // and the candidate with the strongest reason to review is the one it would hide from.
  it('still offers it after a rejection', () => {
    expect(offersDebrief('rejected')).toBe(true);
  });

  it('stays out of the way before anyone has been interviewed', () => {
    for (const stage of ['applied', 'screening', 'responded', 'withdrawn']) {
      expect(offersDebrief(stage), stage).toBe(false);
    }
  });

  // The vocabulary is generated from the Go slice and can drift. An unknown stage is
  // not an interview we know happened, so the offer stays hidden rather than appearing
  // on every application the moment a stage is added.
  it('hides it for a stage it does not know', () => {
    expect(offersDebrief('negotiating')).toBe(false);
    expect(offersDebrief('')).toBe(false);
  });
});

// The selector groups its options so `Closed` reads as a heading over Accepted/Rejected/
// Withdrawn rather than as a fifth state competing with them.
describe('groupedStages', () => {
  it('offers every stage exactly once, under its group', () => {
    const seen = groupedStages().flatMap((g) => g.options.map((o) => o.value));
    expect([...seen].sort()).toEqual([...STAGE_VALUES].sort());
  });

  it('keeps the generated pipeline order', () => {
    expect(groupedStages().map((g) => g.id)).toEqual(['applied', 'interview', 'offer', 'closed']);
  });

  it('labels each option from the generated table', () => {
    const closed = groupedStages().find((g) => g.id === 'closed');
    expect(closed?.label).toBe('Closed');
    // Compared against the generated tables rather than a written-out list: the point of the
    // test is that the labels come from generation, and a literal here would have to be edited
    // every time a stage is added — which is the same coupling it exists to forbid.
    const generated = STAGE_GROUPS.find((g) => g.id === 'closed');
    expect(closed?.options.map((o) => o.value)).toEqual([...generated!.stages]);
    expect(closed?.options.map((o) => o.label)).toEqual(
      generated!.stages.map((s) => STAGE_LABELS[s]),
    );
  });
});
