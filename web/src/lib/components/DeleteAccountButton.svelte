<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { ApiError, api } from '$lib/api';
  import { currentUser } from '$lib/auth.svelte';
  import { locale } from '$lib/i18n/currentLocale.svelte';
  import { t } from '$lib/i18n/t';
  import { consumeReauthDraft } from '$lib/recentAuth';
  import { Button, Dialog, Input } from '$lib/ui';
  import ConfirmIdentity from './ConfirmIdentity.svelte';
  import { messages } from './DeleteAccountButton.messages';

  // Account deletion, stated plainly. This is the one action on the site that cannot
  // be undone on either side — there is no soft-delete, no grace period and no
  // restore — so the warning leads with what disappears and the button is gated behind
  // the member typing their own address. An accidental click costs everything.
  //
  // It is a button that opens a dialog rather than a card on the page: the warning is
  // long by necessity, and a settings tab is no place to keep a wall of red text
  // permanently in view.
  //
  // The server gates this behind a separate, short-lived proof of recent credential
  // control (`recentauth`) on top of the session, so the dialog has to obtain one
  // before it can delete anything. HOW that proof is produced is no longer this
  // component's business: the shared ConfirmIdentity owns it, so all three gated
  // surfaces ask for it the same way and cannot drift apart again.
  //
  // Those provider buttons used to appear only after the server had actually refused,
  // because leaving for a provider destroyed this dialog and everything typed into it,
  // and offering the trip speculatively would have cost the member that for nothing.
  // Carrying the pending action across the trip removed the cost, so the confirmation
  // is simply part of the dialog now.
  let open = $state(false);
  let confirmation = $state('');
  let password = $state('');
  let busy = $state(false);
  let error = $state<string | null>(null);
  let identity = $state<ConfirmIdentity | null>(null);

  // Reopen on return from a provider. The draft carries nothing but its own name: the
  // typed address is NOT restored, because it is not a form field to be helpfully
  // remembered — it is the barrier that stands between a stray click and an irreversible
  // act, and restoring it would quietly lower it.
  $effect(() => {
    if (consumeReauthDraft('delete-account')) open = true;
  });

  // Where this member cancels: the payment provider's own page, asked for rather than
  // composed here. A cancellation flow of our own would be one more thing that can disagree
  // with what actually happened to the money.
  //
  // Fetched when the dialog OPENS, not on page load — it is a call to the provider, the link
  // it returns is short-lived, and most people opening this settings tab are not deleting
  // anything. Null (no subscription, billing off, provider unreachable) simply omits the
  // link: the sentence beside it still says the subscription is not cancelled, and THAT must
  // never depend on a network call succeeding.
  let manageUrl = $state<string | null>(null);

  $effect(() => {
    if (!open) return;
    api
      .billingManageUrl()
      .then(({ url }) => (manageUrl = url))
      .catch(() => (manageUrl = null));
  });

  const s = $derived(t(messages, locale()));
  const user = $derived(currentUser());
  const email = $derived(user?.email ?? '');
  const matches = $derived(confirmation.trim().toLowerCase() === email.toLowerCase() && email !== '');

  // Closing mid-delete would hide the outcome of a request that cannot be repeated,
  // so the dialog holds until the call resolves. `dismissible` is how that is said
  // to Dialog: Escape, the backdrop and the close button go away together while
  // busy, and the platform enforces it rather than a keydown handler here.
  //
  // The reset hangs off `open` rather than off a close() the affordances call,
  // because with Dialog there is no single close path any more — three of them
  // are the platform's.
  $effect(() => {
    if (open) return;
    confirmation = '';
    password = '';
    error = null;
  });

  function messageFor(e: unknown): string {
    if (!(e instanceof ApiError)) return s.genericError;
    // Everything the server says here is worth repeating verbatim — the typed
    // confirmation not matching (400), and the 503 that means nothing was deleted.
    return e.message;
  }

  async function remove() {
    if (!matches || busy) return;
    busy = true;
    error = null;
    try {
      if (!(await identity?.prove())) {
        busy = false;
        return;
      }
      await api.deleteAccount(confirmation.trim());
      // The account is gone and the session cookie with it; re-resolve to signed-out
      // before leaving so no stale user lingers in the layout.
      await invalidateAll();
      await goto(resolve('/'));
    } catch (e) {
      // Identity refusals are worded once, by the component that asked for the proof.
      error = identity?.handleRefusal(e) ?? messageFor(e);
      busy = false;
    }
  }
</script>

<Button
  variant="ghost"
  size="sm"
  class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
  onclick={() => (open = true)}
>
  {s.trigger}
</Button>

<Dialog bind:open title={s.dialogTitle} dismissible={!busy} class="sm:max-w-md border-destructive/30">
  <p class="text-sm text-muted-foreground">{s.warning}</p>

  <ul class="mt-4 flex list-disc flex-col gap-1 pl-5 text-sm text-muted-foreground">
    {#each s.erased as item (item)}
      <li>{item}</li>
    {/each}
  </ul>
  <p class="mt-3 text-sm text-muted-foreground">{s.discussionsNote}</p>
  <!-- Deleting here erases OUR side. The subscription is held by the payment provider,
       which never hears about this, so somebody who does not cancel there keeps paying
       for an account that no longer exists. -->
  <p class="mt-3 text-sm text-muted-foreground">
    {s.subscriptionNote}
    {#if manageUrl}
      <!-- eslint-disable svelte/no-navigation-without-resolve -- the payment provider's own management page, not a SvelteKit route -->
      <a class="underline" href={manageUrl} target="_blank" rel="noopener noreferrer"
        >{s.manageSubscription}</a
      ><!-- eslint-enable svelte/no-navigation-without-resolve -->
    {/if}
  </p>

  <label class="mt-4 block text-sm font-medium" for="delete-account-confirm">
    {s.confirmPrefix} <span class="font-mono">{email}</span> {s.confirmSuffix}
  </label>
  <Input
    id="delete-account-confirm"
    class="mt-1.5"
    autocomplete="off"
    bind:value={confirmation}
    placeholder={email}
    disabled={busy}
  />

  <div class="mt-4">
    <ConfirmIdentity
      bind:this={identity}
      bind:password
      returnTo="/my/security"
      draft={() => ({ surface: 'delete-account' })}
      prompt={s.confirmPrompt}
      active={open}
      disabled={busy}
    />
  </div>

  {#if error}
    <p class="mt-3 text-sm text-destructive">{error}</p>
  {/if}

  <div class="mt-5 flex items-center justify-end gap-2">
    <Button variant="ghost" size="sm" disabled={busy} onclick={() => (open = false)}>{s.cancel}</Button>
    <Button variant="destructive" size="sm" disabled={!matches || busy} onclick={remove}>
      {busy ? s.deleting : s.confirmDelete}
    </Button>
  </div>
</Dialog>
