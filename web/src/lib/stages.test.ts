import { describe, expect, it } from 'vitest';

import { offersDebrief } from './stages';

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
