<script lang="ts">
  import { page } from '$app/state';
  import { currentUser } from '$lib/auth.svelte';
  import CvEditor from '$lib/components/cv/CvEditor.svelte';

  const id = $derived(Number(page.params.id));
  const eligible = $derived(
    currentUser()?.beta_tester === true || currentUser()?.role === 'moderator',
  );
</script>

<svelte:head>
  <title>Edit CV — freehire</title>
</svelte:head>

<div class="max-w-3xl">
  {#if eligible}
    {#key id}
      <CvEditor {id} />
    {/key}
  {:else}
    <p class="text-muted-foreground">The CV builder is in beta and not available on your account yet.</p>
  {/if}
</div>
