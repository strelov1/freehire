<script lang="ts">
  import { invalidateAll } from '$app/navigation';
  import { ApiError, api } from '#lib/api.js';
  import { currentUser } from '#lib/auth.svelte.js';
  import ConfirmIdentity from '#lib/components/ConfirmIdentity.svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { consumeReauthDraft, forgetRecentAuthExpiry } from '#lib/recentAuth.js';
  import { Button, Dialog, Input } from '#lib/ui/index.js';
  import { messages } from './messages';

  // Moving the account to a new address, behind a button, in two steps: the member names
  // the address and
  // confirms it is them (the server gates the request on recent auth), then types the code
  // we mailed to that NEW address. Nothing changes until the code comes back — an address
  // is how the account signs in, so it is switched only once its new holder proves they
  // read it.

  let { onchanged }: { onchanged?: () => void } = $props();

  const s = $derived(t(messages, locale()));
  const current = $derived(currentUser()?.email ?? '');

  let open = $state(false);

  let step = $state<'address' | 'code'>('address');
  let email = $state('');
  let code = $state('');
  let password = $state('');
  let busy = $state(false);
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

  // Back from a provider round trip: the dialog reopens with the address typed before
  // leaving.
  $effect(() => {
    const draft = consumeReauthDraft('change-email');
    if (!draft) return;
    email = draft.email;
    open = true;
  });

  // Closing abandons the attempt; reopening starts from the address. A code already mailed
  // stays valid server-side, so a member who closed by mistake simply asks for a new one.
  // The reset hangs off `open` because Dialog has three close paths of its own.
  $effect(() => {
    if (open) return;
    step = 'address';
    email = code = password = '';
    errorKey = null;
    identityError = null;
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
        // ConfirmIdentity is not mounted on this step, so its `refused()` cannot drop the
        // hint; dropping it here is what makes it ask again when the address step remounts it.
        forgetRecentAuthExpiry();
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
    } catch (err) {
      errorKey = keyFor(err, true);
      busy = false;
      return;
    }
    // Committed: the code is spent, so from here nothing may read as a failed change.
    // The change revoked every session generation, and the recent-auth proof with it.
    forgetRecentAuthExpiry();
    busy = false;
    open = false;
    onchanged?.();
    // Best-effort: re-resolve the session so the new address shows everywhere.
    await invalidateAll().catch(() => {});
  }

  function startOver() {
    resetErrors();
    code = '';
    step = 'address';
  }
</script>

<Button variant="outline" size="sm" onclick={() => (open = true)}>{s.email.change}</Button>

<Dialog bind:open title={s.email.dialogTitle} dismissible={!busy} class="sm:max-w-md">
  <!-- A linked provider resolves by its own identity, never by the address
       (accounts.ResolveOAuthAccount), so a move does not cut it off — said here because
       the fear of losing "Sign in with Google" is what stops somebody changing it. -->
  <p class="mb-4 text-sm text-muted-foreground">{s.email.providersNote}</p>

  {#if step === 'address'}
    <form class="flex flex-col gap-3" onsubmit={sendCode}>
      <label class="flex flex-col gap-1 text-sm">
        <span class="text-muted-foreground">{s.email.newEmailLabel}</span>
        <Input type="email" bind:value={email} required autocomplete="email" disabled={busy} />
      </label>

      <ConfirmIdentity
        bind:this={identity}
        bind:password
        returnTo="/my/account"
        draft={() => ({ surface: 'change-email', email: email.trim() })}
        prompt={s.email.confirmPrompt}
        active={open}
        disabled={busy}
      />

      {#if identityError}
        <p class="text-sm text-destructive">{identityError}</p>
      {:else if errorKey}
        <p class="text-sm text-destructive">{s.email[errorKey]}</p>
      {/if}

      <div class="mt-1 flex items-center justify-end gap-2">
        <Button variant="ghost" size="sm" disabled={busy} onclick={() => (open = false)}>{s.cancel}</Button>
        <Button type="submit" variant="primary" size="sm" disabled={busy || !email.trim()}>
          {busy ? s.email.sending : s.email.sendCode}
        </Button>
      </div>
    </form>
  {:else}
    <form class="flex flex-col gap-3" onsubmit={confirm}>
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

      <div class="flex flex-wrap gap-2">
        <Button variant="ghost" size="sm" disabled={busy} onclick={resend}>{s.email.resend}</Button>
        <Button variant="ghost" size="sm" disabled={busy} onclick={startOver}>{s.email.otherAddress}</Button>
      </div>
      <div class="mt-1 flex justify-end">
        <Button type="submit" variant="primary" size="sm" disabled={busy || !code.trim()}>
          {busy ? s.email.saving : s.email.confirm}
        </Button>
      </div>
    </form>
  {/if}
</Dialog>
