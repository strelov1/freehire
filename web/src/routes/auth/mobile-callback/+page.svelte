<script lang="ts">
  import { page } from '$app/state';

  // Where the mobile v2 OAuth handshake lands. On a phone with the app
  // installed the universal link never reaches this page — iOS and Android hand
  // the URL straight to freehire-mobile, which exchanges the code with the PKCE
  // verifier it alone holds. Reaching the page therefore means the app could not
  // take the link: a desktop browser, or a device without the app.
  //
  // The code is deliberately not exchanged here. It is bound to a verifier that
  // lives only in the app, so a web exchange could not succeed — and echoing it
  // into the page would put a single-use credential into history and referrers.
  const failed = page.url.searchParams.has('auth_error');
</script>

<svelte:head>
  <title>Return to the app · freehire</title>
  <meta name="robots" content="noindex" />
</svelte:head>

<div class="mx-auto max-w-md py-16 text-center">
  {#if failed}
    <h1 class="text-lg font-semibold">Sign-in did not complete</h1>
    <p class="mt-2 text-sm text-muted-foreground">
      Open the freehire app and try signing in again.
    </p>
  {:else}
    <h1 class="text-lg font-semibold">Finishing sign-in…</h1>
    <p class="mt-2 text-sm text-muted-foreground">
      If this page stays open, the freehire app is not installed on this device.
      Sign-in has to finish in the app — open it and try again.
    </p>
  {/if}
</div>
