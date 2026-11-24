<script lang="ts">
  import AccountPageHeading from '#lib/components/AccountPageHeading.svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { announce } from '#lib/announce.svelte.js';
  import { completeProviderLink } from '#lib/providerLink.js';
  import { completeProviderReauthentication, pendingProviderLink } from '#lib/recentAuth.js';

  let error = $state<string | null>(null);
  onMount(async () => {
    const code = page.url.searchParams.get('code');
    // A provider-link trip ends here too (one registered callback); its outcome, good or
    // bad — a refused consent arrives with no code — is shown on the account page it
    // started from.
    if (pendingProviderLink()) {
      const outcome = await completeProviderLink(code);
      const query =
        'linked' in outcome
          ? `linked=${encodeURIComponent(outcome.linked)}`
          : `link_error=${encodeURIComponent(outcome.error)}`;
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() applied to the path; the rule can't see through the appended query string
      await goto(resolve('/my/account') + `?${query}`);
      return;
    }
    if (!code) {
      error = 'The identity check did not complete. Please try again.';
      announce(error, 'assertive');
      return;
    }
    try {
      await goto(resolve(await completeProviderReauthentication(code)));
    } catch {
      error = 'The identity check expired or was already used. Please try again.';
      announce(error, 'assertive');
    }
  });
</script>

<svelte:head><title>Confirm identity · freehire</title></svelte:head>
<div class="mx-auto max-w-md py-16 text-center">
  <AccountPageHeading class="mb-3">Confirm your identity</AccountPageHeading>
  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {:else}
    <p class="text-sm text-muted-foreground">Confirming your identity…</p>
  {/if}
</div>
