<script lang="ts">
  import { resolve } from '$app/paths';
  import { ApiError, api } from '$lib/api';
  import type { TalentNetworkVisibility } from '$lib/types';
  import { Button } from '$lib/ui';

  // Talent Network membership: in, or not. One decision, not three.
  //
  // There used to be an Off / Public / Anonymous picker here. It asked a candidate to
  // reason about a disclosure trade-off at the moment they were least equipped to, and
  // the product can answer it instead: the public catalogue shows nobody's name, nobody's
  // employer and nobody's contact details, so the only question left is whether to be in
  // it. Same GET/PUT /me/talent-network contract as before, two values instead of three.

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  let visibility = $state<TalentNetworkVisibility>('off');
  let handle = $state('');
  // Whether a VISITOR can see them, which membership alone does not answer: a candidate
  // who joins before uploading a CV is a member with a handle whose card still 404s.
  let listed = $state(false);
  // Disables the control while a change is in flight, so a fast double-click cannot race
  // two PUTs.
  let saving = $state(false);
  let saveError = $state<string | null>(null);

  const isMember = $derived(visibility !== 'off');

  $effect(() => {
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
        if (cancelled) return;
        status = 'error';
      }
    })();
    return () => {
      cancelled = true;
    };
  });

  async function setMembership(next: TalentNetworkVisibility) {
    if (next === visibility || saving) return;
    const previous = visibility;
    saving = true;
    saveError = null;
    try {
      // Trust the echoed value, not the click — a rejected PUT must not leave the page
      // showing a state that never saved. The handle comes back on the same response:
      // joining for the first time is what mints it.
      const setting = await api.setTalentNetworkVisibility(next);
      visibility = setting.talent_network_visibility;
      handle = setting.talent_handle ?? '';
      listed = setting.listed;
    } catch (e) {
      visibility = previous;
      saveError =
        e instanceof ApiError ? e.message : 'Could not update your Talent Network setting.';
    } finally {
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>Talent Network — freehire</title>
</svelte:head>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the content readable within the content column. -->
<div class="flex max-w-2xl flex-col gap-4">
  <div class="flex flex-col gap-1">
    <h1 class="text-2xl font-semibold tracking-tight">Talent Network</h1>
    <p class="text-sm text-muted-foreground">
      Get discovered without applying anywhere: appear in a public catalogue recruiters
      browse, without your name on it.
    </p>
  </div>

  {#if status === 'loading'}
    <p class="text-sm text-muted-foreground">Loading…</p>
  {:else if status === 'error'}
    <p class="text-sm text-muted-foreground">Couldn't load this setting.</p>
  {:else}
    {#if saveError}
      <p class="text-sm text-destructive">{saveError}</p>
    {/if}

    <div class="flex flex-col gap-3 rounded-lg border border-border p-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex flex-col gap-0.5">
          <span class="text-sm font-medium text-foreground">
            {isMember ? "You're in the Talent Network" : 'Join the Talent Network'}
          </span>
          <span class="text-xs text-muted-foreground">
            <!-- Three states, not two. Saying "your profile appears in the catalogue" to a
            member the stamp gate excludes is a page telling somebody their profile is up
            while the link beside it 404s. -->
            {#if !isMember}
              Nobody can find you here yet.
            {:else if listed}
              Your profile appears in the public catalogue.
            {:else}
              You're in — but your profile is not shown yet. Upload a CV, or give us a
              moment to finish reading the one you just uploaded.
            {/if}
          </span>
        </div>
        <Button
          variant={isMember ? 'secondary' : 'primary'}
          disabled={saving}
          onclick={() => setMembership(isMember ? 'off' : 'anonymous')}
        >
          {isMember ? 'Leave' : 'Join'}
        </Button>
      </div>

      <!-- The link appears only when the page is actually there. `handle` alone is not
           enough: it is minted on joining, before the CV that makes a card exist. -->
      {#if isMember && listed && handle}
        <!-- Opens the real page in a new tab, so a candidate sees exactly what a visitor
             would. No raw URL and no copy action — the address bar has both once there. -->
        <div>
          <Button
            variant="ghost"
            href={resolve('/talent/[handle]', { handle })}
            target="_blank"
            rel="noopener noreferrer"
          >
            View your public profile
          </Button>
        </div>
      {/if}
    </div>

    <!-- What joining publishes, stated in full and before the decision rather than after
         it. The list is short because the projection is: everything else is withheld. -->
    <div class="flex flex-col gap-2 text-sm text-muted-foreground">
      <p class="font-medium text-foreground">What a visitor sees</p>
      <ul class="list-disc pl-5">
        <li>Your discipline and seniority, and how many years you have worked</li>
        <li>Your skills, and the technologies in each role</li>
        <li>Your city and timezone</li>
      </ul>
      <p class="font-medium text-foreground">What is never shown</p>
      <ul class="list-disc pl-5">
        <li>Your name, photo, email or phone number</li>
        <li>Any employer you have worked for — current or past</li>
        <li>Anything you wrote in your CV in your own words</li>
      </ul>
    </div>

    <p class="text-xs text-muted-foreground">
      Leaving takes your profile down immediately. It cannot un-see what somebody already
      read or saved while it was up.
    </p>
  {/if}
</div>
