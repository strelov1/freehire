<script lang="ts">
  import { onMount } from 'svelte';
  import { Radar, X } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import type { TalentNetworkVisibility } from '$lib/types';
  import { isTalentNetworkMember } from '$lib/talentMembership';
  import { dismissTalentInvite, isTalentInviteDismissed } from '$lib/talentInvite';
  import { Button, Card } from '$lib/ui';

  // The invitation into the Talent Network, mounted by the ACCOUNT shell (`my/+layout`)
  // above whatever section is open — not by Profile's layout, where it used to live.
  // Being found without applying is not a fact about the page a candidate happens to be
  // on, and one who never opens Profile never saw the offer at all.
  //
  // Read-only: it states where the candidate stands and links to the control. Joining is
  // a decision, and a decision belongs on the page that explains what it publishes.
  //
  // Dismissal is permanent and local to the browser. That is only safe because the
  // account navigation carries a Talent Network section of its own now — closing a
  // banner must never be the same gesture as losing the feature.

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  // Starts hidden, so "not yet read from storage" is never mistaken for "not dismissed".
  let dismissed = $state(true);

  onMount(() => {
    dismissed = isTalentInviteDismissed();
  });

  function dismiss() {
    dismissed = true;
    dismissTalentInvite();
  }

  let visibility = $state<TalentNetworkVisibility>('off');
  let handle = $state('');
  // See the settings page: membership does not mean a visitor can see them.
  let listed = $state(false);

  const isMember = $derived(isTalentNetworkMember(visibility));

  $effect(() => {
    // A dismissed candidate is never shown this card, so asking what it would have said
    // buys nothing — and the card now mounts on EVERY `my/*` page, which would turn that
    // into one request per account page load, for good, for someone who closed it.
    // Reading the flag here is what re-runs the effect once `onMount` has answered it,
    // so the order of the two is not something this has to know.
    if (dismissed) return;

    let cancelled = false;
    void (async () => {
      try {
        const setting = await api.getTalentNetwork();
        if (cancelled) return;
        visibility = setting.talent_network_visibility;
        handle = setting.talent_handle ?? '';
        listed = setting.listed;
        status = 'ready';
      } catch {
        // A failed read hides the block rather than showing an error. It is an
        // invitation, not something the page owes the candidate — a red box here would
        // be noise on a page they came to for something else.
        if (cancelled) return;
        status = 'error';
      }
    })();
    return () => {
      cancelled = true;
    };
  });
</script>

{#if status === 'ready' && !dismissed}
  <!-- The gap below the card belongs to the card, not to a wrapper the shell renders
       around it: a wrapper is there whether or not this draws anything, and an empty
       block with a bottom margin is 16px of dead space above every section heading for
       everyone who dismissed this — and for everyone else until the fetch resolves. -->
  <Card class="mb-4 flex flex-wrap items-center justify-between gap-4 p-5">
    <div class="flex min-w-0 items-start gap-3">
      <Radar class="mt-0.5 size-5 shrink-0 text-muted-foreground" aria-hidden="true" />
      <div class="flex min-w-0 flex-col gap-0.5">
        <span class="text-sm font-medium text-foreground">
          {isMember ? "You're in the Talent Network" : 'Get found without applying'}
        </span>
        <span class="text-sm text-muted-foreground">
          {#if !isMember}
            Appear in a public catalogue — your skills and experience, never your name,
            employer or contacts.
          {:else if listed}
            Your anonymous profile is in the public catalogue.
          {:else}
            You're in, but not shown yet — your profile needs a CV we have finished
            reading.
          {/if}
        </span>
      </div>
    </div>

    <div class="flex shrink-0 items-center gap-2">
      {#if isMember && listed && handle}
        <Button
          variant="ghost"
          href={resolve('/talent/[handle]', { handle })}
          target="_blank"
          rel="noopener noreferrer"
        >
          View
        </Button>
      {/if}
      <Button variant={isMember ? 'secondary' : 'primary'} href={resolve('/my/talent-network')}>
        {isMember ? 'Manage' : 'Join'}
      </Button>
      <button
        type="button"
        onclick={dismiss}
        aria-label="Hide this"
        title="Hide this"
        class="-mr-1 rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <X class="size-4" />
      </button>
    </div>
  </Card>
{/if}
