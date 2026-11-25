// Whether the signed-in user already acts for a company (as its claimant or a team member,
// in any status), so the "I'm the employer" offer on job and company pages stays hidden:
// one account per user, a second claim would only be refused. Read once per session from
// GET /api/v1/employer/company, not once per page — the public job and company reads stay
// anonymous. A failed load leaves it false: the offer shows, and following it lands the user
// on their own employer page, which is the harmless way to be wrong.

import { api } from '#lib/api.js';
import { UserResource } from '#lib/userResource.svelte.js';

class EmployerAccount extends UserResource<boolean> {
  has = $state(false);

  protected async load(): Promise<boolean> {
    return (await api.getEmployerCompany()) !== null;
  }

  protected apply(has: boolean) {
    this.has = has;
  }

  protected clearState() {
    this.has = false;
  }

  // Also marks the resource loaded, so a lookup still in flight cannot land afterwards
  // with its older "no account" answer.
  markAccount() {
    this.has = true;
    this.markLoaded();
  }
}

const employerAccount = new EmployerAccount();

export function hasEmployerAccount(): boolean {
  return employerAccount.has;
}

export function ensureEmployerAccountLoaded(): Promise<void> {
  return employerAccount.ensureLoaded();
}

// A claim just went through: the cached "no account" answer is stale, and the next company
// page must not offer a second claim.
export function markEmployerAccount() {
  employerAccount.markAccount();
}
