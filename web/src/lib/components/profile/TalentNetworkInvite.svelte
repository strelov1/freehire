<script lang="ts">
  import { Radar } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { api } from '$lib/api';
  import type { TalentNetworkVisibility } from '$lib/types';
  import { Button, Card } from '$lib/ui';

  // The invitation into the Talent Network, on the profile page.
  //
  // It is here because this is where a candidate finishes describing themselves, which is
  // the moment "be found without applying" is worth offering. The nav entry is the other
  // way in; the feature previously shipped with neither, which is indistinguishable from
  // not having shipped.
  //
  // Read-only: it states where the candidate stands and links to the control. Joining is
  // a decision, and a decision belongs on the page that explains what it publishes.

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let visibility = $state<TalentNetworkVisibility>('off');
  let handle = $state('');

  const isMember = $derived(visibility !== 'off');

  $effect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const setting = await api.getTalentNetwork();
        if (cancelled) return;
        visibility = setting.talent_network_visibility;
        handle = setting.talent_handle ?? '';
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

{#if status === 'ready'}
  <Card class="flex flex-wrap items-center justify-between gap-4 p-5">
    <div class="flex min-w-0 items-start gap-3">
      <Radar class="mt-0.5 size-5 shrink-0 text-muted-foreground" aria-hidden="true" />
      <div class="flex min-w-0 flex-col gap-0.5">
        <span class="text-sm font-medium text-foreground">
          {isMember ? "You're in the Talent Network" : 'Get found without applying'}
        </span>
        <span class="text-sm text-muted-foreground">
          {isMember
            ? 'Your anonymous profile is in the public catalogue.'
            : 'Appear in a public catalogue — your skills and experience, never your name, employer or contacts.'}
        </span>
      </div>
    </div>

    <div class="flex shrink-0 gap-2">
      {#if isMember && handle}
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
    </div>
  </Card>
{/if}
