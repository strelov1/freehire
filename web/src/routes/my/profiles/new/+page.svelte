<script lang="ts">
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import ProfileForm from '$lib/components/ProfileForm.svelte';
  import { Button } from '$lib/ui';
</script>

<svelte:head>
  <title>New profile — freehire</title>
  <!-- Personal page: keep it out of search results. -->
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto w-full max-w-3xl px-4 py-6">
  {#if !isAuthenticated()}
    <div class="flex flex-col items-center gap-3 py-12 text-center">
      <p class="text-sm text-muted-foreground">Sign in to create profiles.</p>
      <Button variant="primary" onclick={() => openAuthDialog()}>Sign in</Button>
    </div>
  {:else}
    <ProfileForm />
  {/if}
</div>
