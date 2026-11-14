<script lang="ts">
  import { resolve } from '$app/paths';
  import { ArrowRight, Target } from '@lucide/svelte';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { promptSignIn } from '#lib/signin.js';
  import { Button } from '#lib/ui/index.js';

  // The job list's notice for a Best-match sort with no skills to rank against: the server
  // serves the newest jobs instead, and this says why and how to get the ranking.
</script>

<!-- `role="status"` because this appears in response to changing the Sort control,
     and a screen-reader user who just did that gets no other signal that the
     ordering they picked was not the one served. -->
<!-- `my-4`, not the nudges' `mt-3`: the feed below sets no top margin of its own,
     so a block sitting between the toolbar and the first job row has to carry the
     air on BOTH sides or it reads as glued to the list. -->
<!-- Laid out like the feed nudges above (icon tile, text, action), and worded as
     what the visitor gets rather than what the sort is missing. -->
<div
  role="status"
  class="my-4 flex flex-col gap-4 rounded-xl border border-brand/30 bg-brand/5 p-4 sm:flex-row sm:items-center sm:p-5"
>
  <div class="flex min-w-0 flex-1 items-start gap-3">
    <span
      class="grid size-10 shrink-0 place-items-center rounded-xl bg-brand-muted text-brand-strong"
      aria-hidden="true"
    >
      <Target class="size-5" />
    </span>
    <div class="min-w-0">
      <p class="font-semibold text-foreground">See your best matches first</p>
      <p class="mt-1 text-sm text-muted-foreground">
        Add your skills or CV and every job is ranked by how well it fits you. Until then, the newest jobs come first.
      </p>
    </div>
  </div>
  <!-- The way out of this state is the whole point of the notice, so it is the
       framed call to action rather than a link trailing the paragraph. -->
  {#if isAuthenticated()}
    <Button variant="cta" href={resolve('/my/profile')} class="w-full shrink-0 sm:w-auto">
      Add your CV
      <ArrowRight class="size-4" aria-hidden="true" />
    </Button>
  {:else}
    <!-- Signed out there is no profile to fill in yet, so the first step is the
         account that would hold one. -->
    <Button variant="cta" onclick={() => promptSignIn()} class="w-full shrink-0 sm:w-auto">
      Sign in to see your matches
      <ArrowRight class="size-4" aria-hidden="true" />
    </Button>
  {/if}
</div>
