<script lang="ts">
  import { Button } from '#lib/ui/index.js';

  // The sign-in offer a signed-out reader meets on a job card's Apply (`show`): sign in, sign
  // up, or open the posting anyway. The live region announces it for the job `title`.
  let {
    show,
    title,
    onSignIn,
    onSignUp,
    onContinue,
  }: { show: boolean; title: string; onSignIn: () => void; onSignUp: () => void; onContinue: () => void } = $props();
</script>

<div class="sr-only" aria-live="polite" aria-atomic="true">
  {#if show}
    Choose how to apply to {title}: sign in, sign up, or continue without signing in.
  {/if}
</div>

{#if show}
  <div
    class="relative z-10 mx-4 mb-3 flex flex-col gap-3 rounded-lg border border-border bg-secondary px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
  >
    <span class="text-sm text-pretty">Sign in to keep track of the jobs you apply to.</span>
    <!-- Two equal buttons over a full-width escape on a phone, where three in a wrapping
       row left the last one stranded on a line of its own; one row from sm up. -->
    <div class="grid grid-cols-2 gap-2 sm:flex sm:shrink-0 sm:items-center">
      <Button variant="primary" onclick={onSignIn}>Sign in</Button>
      <Button variant="outline" onclick={onSignUp}>Sign up</Button>
      <Button variant="ghost" class="col-span-2" onclick={onContinue}>Continue without signing in</Button>
    </div>
  </div>
{/if}
