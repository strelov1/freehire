<script lang="ts">
  import { signinUrl } from '#lib/signin.js';
  import { Button } from '#lib/ui/index.js';

  // What Matches does, for a visitor who is not signed in. Sign up returns to /matches;
  // a new account passes through the onboarding tour first (the layout's own gate).
</script>

<section class="flex flex-col items-center gap-4 py-12 text-center" aria-labelledby="matches-signed-out">
  <h2 id="matches-signed-out" class="text-xl font-semibold">Jobs matched to you, one at a time</h2>
  <ul class="flex max-w-sm list-disc flex-col gap-1.5 pl-5 text-left text-sm text-muted-foreground">
    <li>See why each job fits, what may not, and what's unknown.</li>
    <li>Jobs that are new since your last visit come first.</li>
    <li>Skip, save or apply in one tap, and hide companies you'd never join.</li>
  </ul>
  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin') -->
  <Button variant="cta" size="lg" href={signinUrl({ returnTo: '/matches' })}>Sign up</Button>
</section>
