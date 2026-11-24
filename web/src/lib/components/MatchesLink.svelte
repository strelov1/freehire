<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { PUBLIC_MATCHES } from '$app/env/public';
  import { matchesEnabled } from '#lib/features.js';
  import { MATCHES_LINK } from '#lib/siteNav.js';

  // The way into the match feed, shown to everyone (signed out gets the explainer): a word
  // from `sm`, a bare 44px icon down to 360px, and below that the burger carries it instead.
  const Icon = MATCHES_LINK.icon;
  const on = matchesEnabled({ PUBLIC_MATCHES });
</script>

{#if on}
  <a
    href={resolve(MATCHES_LINK.href)}
    aria-label={MATCHES_LINK.label}
    aria-current={page.url.pathname === MATCHES_LINK.href ? 'page' : undefined}
    class="hidden size-11 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:text-foreground aria-[current=page]:text-foreground min-[360px]:inline-flex sm:size-auto sm:px-2 sm:text-sm"
  >
    <Icon class="size-5 sm:hidden" aria-hidden="true" />
    <span class="hidden sm:inline">{MATCHES_LINK.label}</span>
  </a>
{/if}
