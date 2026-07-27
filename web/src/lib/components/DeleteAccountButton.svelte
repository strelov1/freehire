<script lang="ts">
  import { goto, invalidateAll } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { ApiError, api } from '$lib/api';
  import { focusTrap } from '$lib/actions/focusTrap';
  import { currentUser } from '$lib/auth.svelte';
  import { Button, Input } from '$lib/ui';

  // Account deletion, stated plainly. This is the one action on the site that cannot
  // be undone on either side — there is no soft-delete, no grace period and no
  // restore — so the warning leads with what disappears and the button is gated behind
  // the member typing their own address. An accidental click costs everything.
  //
  // It is a button that opens a dialog rather than a card on the page: the warning is
  // long by necessity, and a settings tab is no place to keep a wall of red text
  // permanently in view.
  let open = $state(false);
  let confirmation = $state('');
  let busy = $state(false);
  let error = $state<string | null>(null);

  const email = $derived(currentUser()?.email ?? '');
  const matches = $derived(confirmation.trim().toLowerCase() === email.toLowerCase() && email !== '');

  // What the member is about to lose, named concretely: "your data" tells them nothing.
  const erased = [
    'Your CV, its parsed profile and every CV you built or tailored',
    'Your hosted mailbox, connected Gmail and all stored messages',
    'Saved jobs, applications, reminders and match analyses',
    'AI credits, saved searches, filters and API keys',
    'Your anonymous community handle',
  ];

  // Closing mid-delete would hide the outcome of a request that cannot be repeated, so
  // the dialog holds until the call resolves.
  function close() {
    if (busy) return;
    open = false;
    confirmation = '';
    error = null;
  }

  async function remove() {
    if (!matches || busy) return;
    busy = true;
    error = null;
    try {
      await api.deleteAccount(confirmation.trim());
      // The account is gone and the session cookie with it; re-resolve to signed-out
      // before leaving so no stale user lingers in the layout.
      await invalidateAll();
      await goto(resolve('/'));
    } catch (e) {
      error =
        e instanceof ApiError
          ? e.message
          : 'Could not delete your account. Nothing was deleted — please try again.';
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
  Delete account
</Button>

<svelte:window onkeydown={(e) => open && e.key === 'Escape' && close()} />

{#if open}
  <div class="fixed inset-0 z-50 flex items-center justify-center p-4">
    <!-- Backdrop is a real button so closing on click is keyboard-accessible. -->
    <button type="button" aria-label="Close dialog" class="absolute inset-0 bg-black/50" onclick={close}
    ></button>

    <div
      role="dialog"
      aria-modal="true"
      aria-label="Delete account"
      class="relative flex max-h-[85vh] w-full max-w-md flex-col overflow-y-auto rounded-lg border border-destructive/30 bg-background p-6 shadow-lg"
      {@attach focusTrap()}
    >
      <h2 class="text-base font-semibold tracking-tight text-destructive">Delete account</h2>
      <p class="mt-1.5 text-sm text-muted-foreground">
        This permanently erases your account and everything in it. It cannot be undone, and
        we cannot restore any of it afterwards.
      </p>

      <ul class="mt-4 flex list-disc flex-col gap-1 pl-5 text-sm text-muted-foreground">
        {#each erased as item (item)}
          <li>{item}</li>
        {/each}
      </ul>
      <p class="mt-3 text-sm text-muted-foreground">
        Discussions you started stay up so other members don't lose their replies, but your
        handle is removed from them — they are shown as written by a deleted member.
      </p>

      <label class="mt-4 block text-sm font-medium" for="delete-account-confirm">
        Type <span class="font-mono">{email}</span> to confirm
      </label>
      <Input
        id="delete-account-confirm"
        class="mt-1.5"
        autocomplete="off"
        bind:value={confirmation}
        placeholder={email}
        disabled={busy}
      />

      {#if error}
        <p class="mt-3 text-sm text-destructive">{error}</p>
      {/if}

      <div class="mt-5 flex items-center justify-end gap-2">
        <Button variant="ghost" size="sm" disabled={busy} onclick={close}>Cancel</Button>
        <Button variant="destructive" size="sm" disabled={!matches || busy} onclick={remove}>
          {busy ? 'Deleting…' : 'Delete account permanently'}
        </Button>
      </div>
    </div>
  </div>
{/if}
