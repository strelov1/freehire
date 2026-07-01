<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import ProfileForm from '$lib/components/ProfileForm.svelte';
  import States from '$lib/components/States.svelte';
  import { searchProfiles } from '$lib/searchProfiles.svelte';
  import { Button } from '$lib/ui';

  const id = $derived(Number(page.params.id));

  let status = $state<'loading' | 'error' | 'ready'>('loading');
  // Resolve the edited profile from the (cached) list — the form needs its current
  // fields to seed itself. Load once the session is confirmed, then look it up.
  const profile = $derived(searchProfiles.items.find((p) => p.id === id));

  async function loadProfiles() {
    status = 'loading';
    try {
      await searchProfiles.ensureLoaded();
      status = 'ready';
    } catch {
      status = 'error';
    }
  }

  $effect(() => {
    if (isAuthenticated()) void loadProfiles();
  });
</script>

<svelte:head>
  <title>Edit profile — freehire</title>
  <!-- Personal page: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  {#if !isAuthenticated()}
    <div class="flex flex-col items-center gap-3 py-12 text-center">
      <p class="text-sm text-muted-foreground">Sign in to edit profiles.</p>
      <Button variant="primary" onclick={() => openAuthDialog()}>Sign in</Button>
    </div>
  {:else if status === 'loading'}
    <States state="loading" />
  {:else if status === 'error'}
    <States state="error" message="Couldn't load your profiles." />
  {:else if profile === undefined}
    <div class="flex flex-col items-center gap-3 py-12 text-center">
      <p class="text-sm text-muted-foreground">This profile no longer exists.</p>
      <Button variant="primary" href={resolve('/my/profiles')}>Back to profiles</Button>
    </div>
  {:else}
    {#key profile.id}
      <ProfileForm {profile} />
    {/key}
  {/if}
</div>
