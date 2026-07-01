// Global controller for the auth dialog, so any component (e.g. a job's Save
// button) can prompt sign-in — not just the TopBar that renders it.
//
// Unlike the current user (per-request, resolved server-side — see
// auth.svelte.ts), dialog visibility is a client-only UI concern: it defaults
// to closed and is only ever mutated from browser interactions, so a
// module-level $state singleton is safe under SSR (it stays closed on the
// server and never leaks across requests).

type Mode = 'login' | 'register';

let open = $state(false);
let mode = $state<Mode>('login');
let error = $state<string | null>(null);
// Where to send the user after a successful sign-in. Set when a guarded page
// bounced them here to sign in (see TopBar), so both password and OAuth login
// return to the shared deep link instead of the home page. null = stay put.
let redirectTo = $state<string | null>(null);

// Getters keep the state read-only from outside; `mode` also gets a setter so
// the dialog's own sign-in/register toggle can two-way bind to it.
export const authDialog = {
  get open() {
    return open;
  },
  get mode() {
    return mode;
  },
  set mode(value: Mode) {
    mode = value;
  },
  get error() {
    return error;
  },
  get redirectTo() {
    return redirectTo;
  },
};

/** Open the dialog (defaults to sign-in), optionally seeding an inline error and
 *  a post-login redirect target (a validated same-origin path). */
export function openAuthDialog(
  initialMode: Mode = 'login',
  initialError: string | null = null,
  redirect: string | null = null,
) {
  mode = initialMode;
  error = initialError;
  redirectTo = redirect;
  open = true;
}

export function closeAuthDialog() {
  open = false;
  redirectTo = null;
}
