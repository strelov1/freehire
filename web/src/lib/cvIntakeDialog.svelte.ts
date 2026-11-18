// Singleton opener for the "tailor for a job" intake dialog, the same shape as
// cvRefreshDialog.svelte.ts. Several places open it — the CV section's header button
// (my/cvs/+layout.svelte), the empty list's own call to action (CvList) and each base card's
// "Tailor to a job" — and the dialog is mounted once, by the layout, so none of them owns the state.

let open = $state(false);
let baseCvId = $state<string | undefined>(undefined);

export const cvIntakeDialog = {
  get open() {
    return open;
  },
  /** The base the dialog was opened for, or undefined when the candidate has not chosen one. */
  get baseCvId() {
    return baseCvId;
  },
};

/** Opens the intake. With `baseCvId` the tailored CV is made from that base and the dialog asks
 *  no question about it; without, a candidate with two or more bases picks inside the dialog. */
export function openCvIntake(base?: string) {
  baseCvId = base;
  open = true;
}

export function closeCvIntake() {
  open = false;
  baseCvId = undefined;
}
