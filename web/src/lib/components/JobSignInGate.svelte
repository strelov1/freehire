<script lang="ts">
  import { Button } from '#lib/ui/index.js';

  // The sign-in offer a signed-out reader meets on the job page's Apply: sign in, sign up
  // or, for a posting applied to elsewhere, continue to it anyway. A vacancy published
  // here (`directApply`) takes its application on this site, so it offers no way around.
  let {
    directApply,
    onsignin,
    onsignup,
    oncontinue,
  }: { directApply: boolean; onsignin: () => void; onsignup: () => void; oncontinue: () => void } = $props();
</script>

<div
  aria-live="polite"
  class="flex flex-col gap-3 rounded-md border border-border bg-secondary px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
>
  <span class="text-sm text-pretty">
    {directApply
      ? 'Sign in to apply — your CV goes straight to the employer.'
      : 'Sign in to keep track of the jobs you apply to.'}
  </span>
  <!-- The same phone layout as the job card's gate (JobRow.svelte). -->
  <div class="grid grid-cols-2 gap-2 sm:flex sm:shrink-0 sm:items-center">
    <Button variant="primary" size="sm" onclick={onsignin}>Sign in</Button>
    <Button variant="outline" size="sm" onclick={onsignup}>Sign up</Button>
    {#if !directApply}
      <Button variant="ghost" size="sm" class="col-span-2" onclick={oncontinue}>Continue without signing in</Button>
    {/if}
  </div>
</div>
