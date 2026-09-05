<script lang="ts">
  // The dedicated credential page: register, sign in, and password recovery, all in
  // one place — every "sign in" entry point in the app funnels here rather than
  // opening a dialog over whatever page it was clicked from. Reached via `signinUrl()`
  // ($lib/signin), which every caller uses so the query params below can't drift:
  // /onboarding's own gate (an anonymous visitor there is bounced straight here,
  // `returnTo` carrying the onboarding URL); every guarded route's server-side load
  // (`redirect(302, ...)` when signed out); TopBar's global handler for a failed OAuth
  // callback (`?error=oauth`, appended by the backend to whatever `returnTo` an OAuth
  // attempt started with — see below); and explicit "Sign in" links/buttons.
  //
  // Which of the four screens is showing lives in the URL hash (#register, #login,
  // #forgot, #reset) as well as in `mode` — every switch writes both, so the address
  // bar always names the screen actually on it. That's what makes "the register
  // form" or "the reset-password form" a real, sharable/bookmarkable/refreshable
  // URL instead of only reachable by clicking through from #login.
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { X } from '@lucide/svelte';
  import { api, ApiError } from '$lib/api';
  import { credentialErrorMessage } from '$lib/credentialErrorMessage';
  import { login, register } from '$lib/auth.svelte';
  import { loadOAuthProviders, PROVIDER_LABELS } from '$lib/oauthProviders';
  import { safeRedirect } from '$lib/safeRedirect';
  import { focusTrap } from '$lib/actions/focusTrap';
  import AuthBrandPanel from '$lib/components/AuthBrandPanel.svelte';
  import BrandMark from '$lib/components/BrandMark.svelte';
  import { Button, ProviderIcon } from '$lib/ui';

  // Validated the same way every other returnTo in the app is (safeRedirect:
  // same-origin relative path only), since it arrives as a query param a visitor's
  // browser holds.
  const returnTo = $derived(safeRedirect(page.url.searchParams.get('returnTo')) ?? '/');

  // Where the close button (and Escape) go — deliberately a SEPARATE param from
  // `returnTo`. When /onboarding sends a visitor here, `returnTo` is /onboarding's
  // own URL (so a successful sign-in/register lands back there and the wizard
  // continues); closing without authenticating must NOT reuse that, or it bounces
  // back to /onboarding, which immediately redirects here again — a loop. Falls
  // back to `returnTo` for any caller that only ever had one destination to begin
  // with (nothing else to skip past).
  const cancelTo = $derived(safeRedirect(page.url.searchParams.get('cancelTo')) ?? returnTo);

  type Mode = 'register' | 'login' | 'forgot' | 'reset';

  // The hash names the screen once landed here (see the header comment), but a
  // fragment never reaches the server — so a caller-requested mode arrives as
  // `?mode=login` instead (see signinUrl()'s own doc comment), which IS visible at
  // SSR time and is what keeps the server-rendered screen from ever showing the
  // wrong form. The hash still wins when both are present (an in-page switch, or a
  // visitor who edited the URL by hand). `?error=oauth` (a failed OAuth callback —
  // see TopBar.svelte's global handler) is always a sign-IN attempt too.
  function initialMode(): Mode {
    switch (page.url.hash.slice(1)) {
      case 'login':
        return 'login';
      case 'forgot':
        return 'forgot';
      case 'reset':
        return 'reset';
      case 'register':
        return 'register';
    }
    if (page.url.searchParams.get('mode') === 'login') return 'login';
    return page.url.searchParams.get('error') === 'oauth' ? 'login' : 'register';
  }
  let mode = $state<Mode>(initialMode());

  // Keeps the URL's hash pointing at whatever screen is actually showing —
  // `replaceState` so switching modes doesn't spam browser history, `noScroll`
  // since the hash names a screen, not an in-page anchor to jump to.
  function syncHash(next: Mode) {
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- a hash-only change on this same page, not a route
    void goto(`#${next}`, { replaceState: true, noScroll: true, keepFocus: true });
  }

  // Canonicalizes the URL on arrival too: a visit with no hash (or a stray one)
  // still ends up on the hash matching the screen it actually shows.
  onMount(() => syncHash(mode));

  let email = $state('');
  let password = $state('');
  let code = $state('');
  // Seeded from `?error=oauth`, not read reactively: the query param is only ever
  // meant to prime the FIRST render, and clearing it must not make it reappear if a
  // later `switchMode()` re-reads it.
  let error = $state<string | null>(
    page.url.searchParams.get('error') === 'oauth' ? 'Sign-in failed. Please try again.' : null,
  );
  let notice = $state<string | null>(null);
  let submitting = $state(false);

  const providerLabels = PROVIDER_LABELS;
  let providers = $state<string[]>([]);
  void loadOAuthProviders().then((names) => (providers = names));

  const titles: Record<Mode, string> = {
    register: 'Create your account',
    login: 'Sign in',
    forgot: 'Reset your password',
    reset: 'Set a new password',
  };
  const isRecovery = $derived(mode === 'forgot' || mode === 'reset');

  function subtitleFor(m: Mode): string {
    if (m === 'register') return "We'll ask for your CV and a few details next.";
    if (m === 'login') return 'Welcome back.';
    if (m === 'reset') return notice ?? 'Enter the code we sent and a new password.';
    return "We'll email you a code to reset your password.";
  }
  const subtitle = $derived(subtitleFor(mode));

  function messageFor(e: unknown): string {
    if (e instanceof ApiError) {
      if (mode === 'register' && e.status === 409) return 'That email is already registered — sign in instead.';
      if (e.status === 429) return 'A code was just sent — check your inbox.';
      if (e.status === 503) return 'Email delivery is unavailable right now.';
      if (isRecovery && e.status === 400) return 'That code is not valid or has expired, or the password is too short.';
    }
    return credentialErrorMessage(e) ?? 'Something went wrong. Please try again.';
  }

  // Ask for a reset code. The server answers the same way whether or not the address
  // has an account, so the UI must not imply that a code is on its way to a real one.
  async function requestReset() {
    error = null;
    submitting = true;
    try {
      await api.forgotPassword(email);
      notice = 'If that address has an account, a code is on its way. It expires in 15 minutes.';
      mode = 'reset';
      syncHash('reset');
    } catch (err) {
      error = messageFor(err);
    } finally {
      submitting = false;
    }
  }

  // Set the new password, then sign in with it: the reset revokes every session, so
  // there is nothing to carry over.
  async function completeReset() {
    error = null;
    submitting = true;
    try {
      await api.resetPassword(email, code.trim(), password);
      await login(email, password);
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- returnTo is a validated same-origin path (safeRedirect), not a typed route
      void goto(returnTo);
    } catch (err) {
      error = messageFor(err);
    } finally {
      submitting = false;
    }
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (mode === 'forgot') return requestReset();
    if (mode === 'reset') return completeReset();
    error = null;
    submitting = true;
    try {
      await (mode === 'register' ? register : login)(email, password);
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- returnTo is a validated same-origin path (safeRedirect), not a typed route
      void goto(returnTo);
    } catch (err) {
      error = messageFor(err);
    } finally {
      submitting = false;
    }
  }

  // Every mode change clears whatever the previous step was saying, so a stale error
  // or notice never bleeds into the next screen — and updates the hash to match.
  function switchMode(next: Mode) {
    mode = next;
    error = null;
    notice = null;
    syncHash(next);
  }

  function leave() {
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- cancelTo is a validated same-origin path (safeRedirect), not a typed route
    void goto(cancelTo);
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') leave();
  }

  const inputClass =
    'rounded-md border border-border bg-background px-4 py-3 text-base focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring';

  // The size every full-width button in this column shares — the provider buttons
  // and the submit button — so the choices read as equally weighted. Named once so
  // the two cannot drift apart; only their fill and weight differ.
  const buttonSizeClass = 'h-12 rounded-lg px-5 text-base';
</script>

<svelte:head>
  <title>{titles[mode]} · freehire</title>
</svelte:head>

<svelte:window onkeydown={onKeydown} />

<div
  class="fixed inset-0 z-50 flex bg-background"
  role="dialog"
  aria-modal="true"
  aria-label={titles[mode]}
  {@attach focusTrap()}
>
  <AuthBrandPanel />

  <div class="no-scrollbar relative flex flex-1 flex-col overflow-y-auto">
    <button
      type="button"
      onclick={leave}
      aria-label="Close"
      class="absolute right-5 top-5 flex size-8 items-center justify-center rounded-lg border border-border text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
    >
      <X class="size-4" />
    </button>

    <div class="m-auto flex w-full max-w-sm flex-col gap-6 px-5 py-16">
      <div class="lg:hidden">
        <a href={resolve('/')} class="flex items-center gap-2 text-sm font-semibold tracking-tight">
          <BrandMark />
          freehire
        </a>
      </div>

      <div>
        <h1 class="text-xl font-semibold tracking-tight">{titles[mode]}</h1>
        <p class="mt-1 text-sm text-muted-foreground">{subtitle}</p>
      </div>

      {#if providers.length > 0 && !isRecovery}
        <div class="flex flex-col gap-3">
          {#each providers as provider (provider)}
            <Button
              variant="outline"
              class={buttonSizeClass}
              href={`/api/v1/auth/oauth/${provider}/start?returnTo=${encodeURIComponent(returnTo)}`}
            >
              <ProviderIcon {provider} class="size-5" />
              Continue with {providerLabels[provider]}
            </Button>
          {/each}
        </div>
        <div class="flex items-center gap-3 text-xs text-muted-foreground">
          <span class="h-px flex-1 bg-border"></span>
          or
          <span class="h-px flex-1 bg-border"></span>
        </div>
      {/if}

      <form class="flex flex-col gap-3" onsubmit={submit}>
        <label class="flex flex-col gap-1 text-sm">
          <span class="text-muted-foreground">Email</span>
          <input
            type="email"
            bind:value={email}
            required
            autocomplete="email"
            class={inputClass}
          />
        </label>

        {#if mode === 'reset'}
          <label class="flex flex-col gap-1 text-sm">
            <span class="text-muted-foreground">Code from the email</span>
            <input
              bind:value={code}
              inputmode="numeric"
              autocomplete="one-time-code"
              maxlength={6}
              required
              class={[inputClass, 'tracking-widest']}
            />
          </label>
        {/if}

        {#if mode !== 'forgot'}
          <label class="flex flex-col gap-1 text-sm">
            <span class="text-muted-foreground">{mode === 'reset' ? 'New password' : 'Password'}</span>
            <input
              type="password"
              bind:value={password}
              required
              minlength={mode === 'login' ? undefined : 8}
              autocomplete={mode === 'login' ? 'current-password' : 'new-password'}
              class={inputClass}
            />
          </label>
        {/if}

        {#if error}
          <p class="text-sm text-destructive">{error}</p>
        {/if}

        <button
          type="submit"
          disabled={submitting}
          class={[
            buttonSizeClass,
            'mt-1 inline-flex items-center justify-center bg-brand font-semibold text-brand-foreground transition-opacity hover:opacity-90 disabled:opacity-60',
          ]}
        >
          {submitting ? 'Please wait…' : titles[mode]}
        </button>
      </form>

      {#if mode === 'login'}
        <p class="text-center text-sm">
          <button type="button" onclick={() => switchMode('forgot')} class="text-muted-foreground underline-offset-4 hover:underline">
            Forgot your password?
          </button>
        </p>
      {/if}

      {#if isRecovery}
        <p class="text-center text-sm text-muted-foreground">
          <button
            type="button"
            onclick={() => switchMode('login')}
            class="font-medium text-foreground underline-offset-4 hover:underline"
          >
            Back to sign in
          </button>
        </p>
      {:else}
        <p class="text-center text-sm text-muted-foreground">
          {mode === 'register' ? 'Already have an account?' : "Don't have an account?"}
          <button
            type="button"
            onclick={() => switchMode(mode === 'register' ? 'login' : 'register')}
            class="font-medium text-foreground underline-offset-4 hover:underline"
          >
            {mode === 'register' ? 'Sign in' : 'Create one'}
          </button>
        </p>
      {/if}
    </div>
  </div>
</div>

<style>
  /* Same pattern as /onboarding's step content: scrolls without a visible
     scrollbar, so the form panel reads as a page rather than a scroll pane
     with a rail. */
  .no-scrollbar {
    scrollbar-width: none;
    -ms-overflow-style: none;
  }
  .no-scrollbar::-webkit-scrollbar {
    display: none;
  }
</style>
