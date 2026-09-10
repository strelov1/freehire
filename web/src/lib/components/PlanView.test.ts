import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, deliberately not a mounted-component test — web/ has no
// component-test infrastructure at all (see jobActionStrip.test.ts's own comment: no
// Svelte plugin, no DOM in vitest.config.ts).
//
// What is worth pinning here: the "Upgrade to Ultra" call to action must not advertise a
// tier that is not actually for sale. A Pro subscriber unconditionally saw it before this
// changed — the CTA always led to /pricing regardless of whether Ultra had a price to
// show there. The regression this guards against is a later edit dropping the
// `ultraOffered` half of the condition and going back to that.
const SOURCE = readFileSync(join(import.meta.dirname, 'PlanView.svelte'), 'utf8');

describe('PlanView upgrade CTA', () => {
  it("only shows 'Upgrade to Ultra' to a Pro subscriber when Ultra is offered", () => {
    expect(SOURCE).toContain("plan.plan === 'free' || (plan.plan === 'pro' && ultraOffered)");
  });

  it('derives whether Ultra is offered from the same public prices list /pricing renders its column from', () => {
    // One source of truth about what is for sale, rather than a second signal (e.g. a
    // boolean baked into PlanState) that could drift from the actual prices endpoint.
    expect(SOURCE).toContain("m.prices.some((p) => p.tier === 'ultra')");
  });

  it('only fetches the prices list for a Pro account, not on every visit', () => {
    expect(SOURCE).toContain("if (plan?.plan !== 'pro') return;");
  });
});
