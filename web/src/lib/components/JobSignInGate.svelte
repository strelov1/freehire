<script lang="ts">
  import { promptLogin, promptSignIn } from '#lib/signin.js';
  import { Button } from '#lib/ui/index.js';

  // The sign-in offer a signed-out reader meets on the job page's Apply: sign in, sign up
  // or, for a posting applied to elsewhere, continue to it anyway. A vacancy published
  // here (`directApply`) takes its application on this site, so it offers no way around.
  let {
    directApply,
    url,
    onclose,
  }: {
    directApply: boolean;
    /** The posting's own page, for "Continue without signing in". */
    url: string;
    /** Called as any choice is made: the gate has done its job. */
    onclose: () => void;
  } = $props();

  // "Sign up" enters /onboarding first, which returns to this posting (it can be skipped
  // straight to the sign-up form); "Sign in" is the explicit sign-in form; "Continue
  // without signing in" opens the posting in a new tab (the navigation the click was
  // holding back).
  function signUp() {
    onclose();
    promptSignIn();
  }

  function signIn() {
    onclose();
    promptLogin();
  }

  function continueToPosting() {
    onclose();
    window.open(url, '_blank', 'noopener,noreferrer');
  }
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
    <Button variant="primary" size="sm" onclick={signIn}>Sign in</Button>
    <Button variant="outline" size="sm" onclick={signUp}>Sign up</Button>
    {#if !directApply}
      <Button variant="ghost" size="sm" class="col-span-2" onclick={continueToPosting}
        >Continue without signing in</Button
      >
    {/if}
  </div>
</div>
