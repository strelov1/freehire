<script lang="ts">
  import { invalidateAll } from '$app/navigation';
  import { ApiError, api } from '#lib/api.js';
  import { currentUser } from '#lib/auth.svelte.js';
  import ConfirmIdentity from '#lib/components/ConfirmIdentity.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { consumeReauthDraft, forgetRecentAuthExpiry } from '#lib/recentAuth.js';
  import { Button, Input } from '#lib/ui/index.js';
  import { messages } from './messages';

  // Moving the account to a new address, in two steps: the member names the address and
  // confirms it is them (the server gates the request on recent auth), then types the code
  // we mailed to that NEW address. Nothing changes until the code comes back — an address
  // is how the account signs in, so it is switched only once its new holder proves they
  // read it.

  const s = $derived(t(messages, locale()));
  const current = $derived(currentUser()?.email ?? '');

  let step = $state<'address' | 'code'>('address');
  let email = $state('');
  let code = $state('');
  let password = $state('');
  let busy = $state(false);
  let changed = $state(false);
  let identity = $state<ConfirmIdentity | null>(null);

  type ErrorKey =
    | 'invalidEmail'
    | 'sameEmail'
    | 'emailTaken'
    | 'tooSoon'
    | 'invalidCode'
    | 'unavailable'
    | 'genericError';
  // The KEY, not the string, so a language switch re-renders a visible error.
  let errorKey = $state<ErrorKey | null>(null);
  // ConfirmIdentity words its own refusals (428, a wrong password) in its own vocabulary.
  let identityError = $state<string | null>(null);

  // Back from a provider round trip: the address typed before leaving comes back with it.
  $effect(() => {
    const draft = consumeReauthDraft('change-email');
    if (draft) email = draft.email;
  });

  function keyFor(e: unknown, onCode: boolean): ErrorKey {
    if (e instanceof ApiError) {
      if (e.status === 400) return onCode ? 'invalidCode' : 'invalidEmail';
      if (e.status === 409) return 'emailTaken';
      if (e.status === 429) return 'tooSoon';
      if (e.status === 503) return 'unavailable';
    }
    return 'genericError';
  }

  function resetErrors() {
    errorKey = null;
    identityError = null;
  }

  async function sendCode(e?: SubmitEvent) {
    e?.preventDefault();
    if (busy) return;
    resetErrors();
    changed = false;
    // Answered here rather than by a round trip: the server's own 400 for it is
    // indistinguishable from a malformed address.
    if (email.trim().toLowerCase() === current.toLowerCase()) {
      errorKey = 'sameEmail';
      return;
    }
    busy = true;
    try {
      if (!(await identity?.prove())) return;
      await api.requestEmailChange(email.trim());
      password = '';
      code = '';
      step = 'code';
    } catch (err) {
      identityError = identity?.handleRefusal(err) ?? null;
      if (!identityError) errorKey = keyFor(err, false);
    } finally {
      busy = false;
    }
  }

  async function resend() {
    if (busy) return;
    resetErrors();
    busy = true;
    try {
      await api.requestEmailChange(email.trim());
    } catch (err) {
      // The resend is gated like the first request; a proof that lapsed meanwhile sends the
      // member back to the first step, where ConfirmIdentity can ask again.
      if (err instanceof ApiError && err.status === 428) {
        identity?.refused();
        step = 'address';
      }
      errorKey = keyFor(err, false);
    } finally {
      busy = false;
    }
  }

  async function confirm(e: SubmitEvent) {
    e.preventDefault();
    if (busy) return;
    resetErrors();
    busy = true;
    try {
      await api.confirmEmailChange(code.trim());
      // The change revoked every session generation, and the recent-auth proof with it.
      forgetRecentAuthExpiry();
      await invalidateAll();
      changed = true;
      step = 'address';
      email = code = '';
    } catch (err) {
      errorKey = keyFor(err, true);
    } finally {
      busy = false;
    }
  }

  function startOver() {
    resetErrors();
    code = '';
    step = 'address';
  }
</script>

<section class="mb-8 rounded-lg border border-border p-4">
  <h2 class="mb-1 text-sm font-semibold">{s.email.heading}</h2>
  <p class="mb-4 text-sm text-muted-foreground">
    {s.email.currentPrefix} <span class="font-medium text-foreground">{current}</span>
  </p>

  {#if step === 'address'}
    <form class="flex max-w-sm flex-col gap-3" onsubmit={sendCode}>
      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">{s.email.newEmailLabel}</span>
        <Input type="email" bind:value={email} required autocomplete="email" disabled={busy} />
      </label>

      <ConfirmIdentity
        bind:this={identity}
        bind:password
        returnTo="/my/security"
        draft={() => ({ surface: 'change-email', email: email.trim() })}
        prompt={s.email.confirmPrompt}
        disabled={busy}
      />

      {#if identityError}
        <p class="text-sm text-destructive">{identityError}</p>
      {:else if errorKey}
        <p class="text-sm text-destructive">{s.email[errorKey]}</p>
      {/if}
      {#if changed}
        <p class="text-sm text-muted-foreground">{s.email.changed}</p>
      {/if}

      <Button type="submit" variant="primary" disabled={busy || !email.trim()} class="mt-1">
        {busy ? s.email.sending : s.email.sendCode}
      </Button>
    </form>
  {:else}
    <form class="flex max-w-sm flex-col gap-3" onsubmit={confirm}>
      <p class="text-sm">
        {s.email.codeSentPrefix} <span class="font-medium">{email.trim()}</span>{s.email.codeSentSuffix}
      </p>
      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">{s.email.codeLabel}</span>
        <Input
          bind:value={code}
          required
          inputmode="numeric"
          autocomplete="one-time-code"
          maxlength={6}
          disabled={busy}
        />
      </label>

      {#if errorKey}
        <p class="text-sm text-destructive">{s.email[errorKey]}</p>
      {/if}

      <Button type="submit" variant="primary" disabled={busy || !code.trim()} class="mt-1">
        {busy ? s.email.saving : s.email.confirm}
      </Button>
      <div class="flex flex-wrap gap-2">
        <Button variant="ghost" size="sm" disabled={busy} onclick={resend}>{s.email.resend}</Button>
        <Button variant="ghost" size="sm" disabled={busy} onclick={startOver}>{s.email.otherAddress}</Button>
      </div>
    </form>
  {/if}
</section>
