<script lang="ts">
  import { ApiError, api } from '$lib/api';
  import { AsyncData } from '$lib/asyncData.svelte';
  import { currentUser } from '$lib/auth.svelte';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import {
    beginProviderReauthentication,
    forgetRecentAuthExpiry,
    recentAuthExpiry,
    type ReauthDraft,
    type ReauthReturnPath,
  } from '$lib/recentAuth';
  import { Button, Input } from '$lib/ui';
  import { messages } from './ConfirmIdentity.messages';

  // The single confirmation surface for every action gated on recent credential control.
  // It exists because the same block was written three times and the three copies drifted:
  // one of them ended up demanding a password inside a modal whose only password input was
  // on the page behind it, which made revoking a key impossible rather than merely awkward.
  //
  // It owns the whole question of HOW a member proves themselves — which method their
  // account even has, fetching the providers only when they are the answer, and whether a
  // proof is already held. The caller owns only its own action: it reads `password` and
  // spends it, asks `isConfirmed()` whether it needs to, and calls `refused()` on a 428.
  //
  // INSIDE A DIALOG, PASS `active={open}`. `design-system/src/dialog.svelte` renders its
  // children unconditionally, so a dialog's contents mount on page load: without this the
  // component would fetch the member's providers on every visit to a page whose dialog they
  // never open, and tick its countdown against a hidden node for the life of the page.
  // It is a prop rather than a `{#if}` at each host so that forgetting it is impossible
  // rather than merely documented.
  let {
    password = $bindable(''),
    returnTo,
    draft,
    prompt,
    active = true,
    disabled = false,
  }: {
    /** The typed password, for the caller to spend on `reauthenticatePassword` immediately
     *  before its own request. Empty and unused for an account with no password. */
    password?: string;
    returnTo: ReauthReturnPath;
    /** What to carry across the provider round trip, read at the moment the member leaves
     *  rather than at mount — the field they are protecting is usually still being typed.
     *  A surface with nothing worth carrying passes nothing. */
    draft?: () => ReauthDraft;
    /** Why THIS action needs confirming. An API key and a deleted account are not dangerous
     *  for the same reason, so the sentence belongs to the caller, not here. */
    prompt: string;
    /** False while the surface holding this is not on screen: nothing is fetched, nothing
     *  ticks, and nothing renders. */
    active?: boolean;
    disabled?: boolean;
  } = $props();

  const s = $derived(t(messages, locale()));
  // Whether a password exists has to come from the server — being signed in says nothing
  // about it, and an OAuth-only account has none to offer. Read from the cached session
  // rather than from `connectedIdentities()`, which also carries `has_password`: taking it
  // from there would mean a request for every password account, which is the one shape
  // that never needs one. Deliberate — not an oversight to "fix" later.
  const hasPassword = $derived(currentUser()?.has_password ?? false);

  // Not `$derived`: this is a snapshot of a cookie the client cannot read, and it must be
  // droppable on demand when the server disagrees with it (see `refused`).
  let expiresAt = $state<Date | null>(recentAuthExpiry());
  // The component's own validation, shown beside the input it is about.
  let inlineError = $state<string | null>(null);
  const confirmed = $derived(expiresAt !== null);

  // Fetched only for the accounts that can actually use a provider, and only while a proof
  // is not already held — a confirmed member is shown no controls to fetch options for.
  const identitiesData = new AsyncData<string[]>([]);
  function loadProviders(): void {
    void identitiesData.run(async () =>
      (await api.connectedIdentities()).identities
        .filter((i) => i.status === 'active')
        .map((i) => i.provider),
    );
  }
  $effect(() => {
    if (!active || hasPassword || confirmed) return;
    loadProviders();
  });

  // A surface that closes and reopens should not greet the member with the complaint it
  // raised last time.
  $effect(() => {
    if (!active) inlineError = null;
  });

  /** Called by the caller when the server answered a gated action with 428 despite this
   *  component reporting a held proof. The server is the authority; the hint goes. */
  export function refused(): void {
    forgetRecentAuthExpiry();
    expiresAt = null;
  }

  /** Whether a proof is already held, so no input is being asked for.
   *
   *  A caller cannot infer this from `password` being empty: that is also what a member
   *  who simply has not typed yet looks like. Exposed so a caller can word its own `401`
   *  correctly — "wrong password" is the wrong thing to tell a confirmed member. */
  export function isConfirmed(): boolean {
    return confirmed;
  }

  /** Make sure a proof is held. Called immediately before the gated request; the caller
   *  proceeds only on `true`.
   *
   *  A held proof is spent as-is; otherwise the typed password buys one. This lives here
   *  rather than at each call site because the decision is identical at all three of them,
   *  and three copies of one decision is precisely what produced three different
   *  behaviours from one requirement before this component existed.
   *
   *  `false` means the member has not supplied what was asked for — and the reason is
   *  already on screen, beside the control it refers to. A blank password is NOT sent:
   *  `''` earns a 401, which the caller would word as "that password is not right" to
   *  somebody who has not typed one.
   *
   *  A password the server refuses still throws; `handleRefusal` words that. */
  export async function prove(): Promise<boolean> {
    if (confirmed) return true;
    if (hasPassword && !password) {
      inlineError = s.passwordRequired;
      return false;
    }
    inlineError = null;
    await api.reauthenticatePassword(password);
    return true;
  }

  /** What to tell the member when the server refused, or null when the refusal was not
   *  about identity and belongs to the caller's own vocabulary.
   *
   *  It also drops a proof the server just overruled, because there is no case where a
   *  surface should report a 428 and go on believing it is confirmed. Both halves live
   *  here because all three callers had grown the same cascade — the very shape this
   *  component exists to stop. */
  export function handleRefusal(e: unknown): string | null {
    if (!(e instanceof ApiError)) return null;
    if (e.status === 428) {
      refused();
      return s.refused;
    }
    // Only when a password was actually asked for: a 401 told to a member who never saw a
    // password box is not about their password.
    if (e.status === 401 && hasPassword && !confirmed) return s.wrongPassword;
    return null;
  }

  // A bare clock, no word beside it — see the note in the message catalogue. Ticks so the
  // claim does not sit there frozen while the proof quietly expires under it, and drops
  // the claim outright once it has: a stale green tick is worse than no tick.
  let now = $state(Date.now());
  $effect(() => {
    if (!active || !confirmed) return;
    const timer = setInterval(() => {
      now = Date.now();
      if (expiresAt && expiresAt.getTime() <= now) expiresAt = null;
    }, 1000);
    return () => clearInterval(timer);
  });
  const remaining = $derived.by(() => {
    if (!expiresAt) return '';
    const seconds = Math.max(0, Math.round((expiresAt.getTime() - now) / 1000));
    return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;
  });

  const providers = $derived(identitiesData.value);
  // The one state this component must never reach in silence: claiming confirmation is
  // required while offering nothing to press.
  const noMethodAvailable = $derived(
    !hasPassword && identitiesData.status === 'ready' && providers.length === 0,
  );
</script>

{#if active}
  <!-- `aria-live` because this block SWAPS IN PLACE: `refused()` turns a standing "identity
     confirmed" back into a password field after the server 428s, and a screen-reader user
     would otherwise be given no sign that what they are looking at changed. -->
{#if confirmed}
  <p
    aria-live="polite"
    class="flex items-center gap-2 rounded-lg border border-border bg-secondary/40 px-3 py-2 text-sm"
  >
    <span aria-hidden="true">✓</span>
    <span>{s.confirmed}</span>
    <span class="text-muted-foreground">· {remaining}</span>
  </p>
{:else}
  <div aria-live="polite" class="flex flex-col gap-2 border-t border-border pt-4">
    <!-- The heading and the prompt are the DEMAND, so they wait until there is something
         to demand with. While the providers are still loading the block says only what it
         is doing: a "confirm it is you" above an empty box is the shape the spec forbids,
         and it is on screen for as long as that request takes. -->
    {#if !hasPassword && identitiesData.status === 'loading'}
      <p class="text-sm text-muted-foreground">{s.providersLoading}</p>
    {:else}
      <p class="text-sm font-medium">{s.heading}</p>
      <p class="text-sm text-muted-foreground">{prompt}</p>

      {#if hasPassword}
        <label class="mt-1 flex flex-col gap-1">
          <span class="text-sm font-medium">{s.passwordLabel}</span>
          <Input type="password" autocomplete="current-password" bind:value={password} {disabled} />
        </label>
        {#if inlineError}
          <p class="text-sm text-destructive">{inlineError}</p>
        {/if}
      {:else if identitiesData.status === 'error' || noMethodAvailable}
      <!-- Both of these states used to be terminal: a sentence, and nothing to press. The
           effect has no reason to re-run, so a member whose network blipped was stuck
           until they reloaded the page — the same dead end this component exists to
           abolish, just one layer down. The retry also covers the no-method case, where
           an empty list may simply be a read that went wrong. -->
        <p class="text-sm text-destructive">
          {identitiesData.status === 'error' ? s.providersError : s.noMethod}
        </p>
        <div class="mt-1">
          <Button variant="outline" size="sm" {disabled} onclick={loadProviders}>{s.retry}</Button>
        </div>
      {:else}
        <p class="text-sm text-muted-foreground">{s.returnNote}</p>
        <div class="mt-1 flex flex-wrap gap-2">
          {#each providers as provider (provider)}
            <Button
              variant="outline"
              size="sm"
              {disabled}
              onclick={() => beginProviderReauthentication(provider, returnTo, draft?.())}
            >
              {s.confirmWithPrefix}
              {provider}
            </Button>
          {/each}
        </div>
      {/if}
    {/if}
  </div>
{/if}
{/if}
