<script lang="ts">
  import { onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import { announce } from '#lib/announce.svelte.js';
  import { Button } from '#lib/ui/index.js';

  // The profile has nothing to match on (no CV, no usable fields, e.g. the tour was skipped).
  let { focus = true }: { focus?: boolean } = $props();
  let heading = $state<HTMLElement | null>(null);
  const title = 'Tell us about yourself — upload a CV or fill in your profile';

  onMount(() => {
    if (!focus) return;
    heading?.focus();
    announce(title);
  });
</script>

<div class="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-12 text-center">
  <h2 bind:this={heading} tabindex="-1" class="max-w-sm text-lg font-semibold outline-none">{title}</h2>
  <Button variant="cta" size="lg" class="mt-2" href={resolve('/my/profile/skills')}>Edit profile</Button>
</div>
