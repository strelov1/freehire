// Singleton opener for the "tailor for a job" intake dialog, the same shape as
// cvRefreshDialog.svelte.ts. Two places open it — the CV section's header button
// (my/cvs/+layout.svelte) and the empty list's own call to action (CvList) — and the
// dialog is mounted once, by the layout, so neither of them owns the state.

let open = $state(false);

export const cvIntakeDialog = {
  get open() {
    return open;
  },
};

export function openCvIntake() {
  open = true;
}

export function closeCvIntake() {
  open = false;
}
