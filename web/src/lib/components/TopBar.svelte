<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { afterNavigate, goto } from '$app/navigation';
  import { signinUrl } from '$lib/signin';
  import HeaderSearch from './HeaderSearch.svelte';
  import HeaderMenu from './HeaderMenu.svelte';
  import BrandMark from './BrandMark.svelte';
  import { isFullBleedRoute } from '$lib/shellLayout';
  import { HEADER_LINKS } from '$lib/siteNav';

  // The header is three slots — logo | search | menu — identical on every
  // viewport. Nav links, the account items, the theme toggle, and the auth
  // action all live in HeaderMenu.
  //
  // The middle slot is ONE search box, on every page but one. Where a list has
  // registered itself (the feed at /jobs, /companies, and a company's or collection's
  // own scoped jobs list) it filters that list in place and hosts the All-filters
  // trigger; everywhere else the same box navigates to the feed carrying the same
  // filter.
  //
  // `listKind` survives only to word the placeholder — the box asks the page nothing
  // else. It used to select between two components that shared everything but that one
  // behaviour and drifted apart in the parts they shared.
  const listKind = $derived(page.url.pathname === '/companies' ? 'companies' : 'jobs');

  // The homepage is the one exception: its whole content is a large centred copy of
  // this same box, so the header renders none. Two identical fields on one screen make
  // the visitor choose between them for no reason, and the hero is the focused one.
  // The brand and the menu stay — sign-in and notifications live in the menu's own
  // control strip, and both have to remain reachable.
  const bareHeader = $derived(page.url.pathname === '/');

  // On the full-viewport surfaces (the agent, the tailor workspace) the page below runs
  // edge to edge under its own icon rail, so the header drops the centered `max-w-6xl`
  // and does the same: brand hard left, menu hard right. The search keeps a readable
  // width and centers itself in the gap rather than stretching across the monitor.
  const fullBleed = $derived(isFullBleedRoute(page.url.pathname));

  // A failed OAuth callback lands back wherever that attempt's `returnTo` pointed,
  // carrying `?auth_error` (appended by the backend — see internal/api/handler/oauth.go)
  // — which could be ANY page, not just one that itself knows about /signin. This is
  // the one place that catches it regardless of where it landed, and sends the visitor
  // on to /signin with the failure surfaced there instead of opening the in-place
  // dialog over whatever page that happened to be. Runs in afterNavigate — not
  // onMount — because this header lives in the persistent root layout: a redirect
  // that lands here via client-side navigation never remounts it, so onMount would
  // fire only on a cold load and miss the in-app bounce. afterNavigate covers both
  // the initial load and every later navigation, and stays off the SSR path.
  afterNavigate(() => {
    if (!page.url.searchParams.has('auth_error')) return;
    const query = [...page.url.searchParams]
      .filter(([key]) => key !== 'auth_error')
      .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`)
      .join('&');
    const returnTo = page.url.pathname + (query ? `?${query}` : '');
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query
    void goto(signinUrl({ returnTo, error: 'oauth' }), { replaceState: true });
  });
</script>

<!-- Solid (not backdrop-blur) on purpose: a backdrop-filter here would become the
     containing block for the mobile menu's `position: fixed` full-screen drawer,
     pinning it to the header instead of the viewport and breaking it. -->
<header class="sticky top-0 z-40 border-b border-border bg-background">
  <div
    class={[
      'mx-auto flex h-14 items-center gap-3 px-4 sm:gap-4',
      fullBleed ? 'max-w-none' : 'max-w-6xl',
    ]}
  >
    <!-- Full-bleed only: the two side slots grow from a zero basis, so the free space
         splits evenly between them and the search sits on the container's axis — the
         menu cluster is wider than the brand, so centering the middle slot on its own
         leftover space would push it visibly off-centre. `basis-0` also keeps them from
         shrinking (shrink scales the basis), so the narrow layout is untouched. -->
    <div class={['flex shrink-0 items-center', fullBleed && 'flex-1 basis-0']}>
      <a
        href={resolve('/')}
        aria-label="freehire"
        class="flex items-center gap-2 text-sm font-semibold tracking-tight"
      >
        <BrandMark />
        <span class="hidden sm:inline" aria-hidden="true">freehire</span>
      </a>
    </div>

    <!-- The slot, not the search component, owns the middle width: each search root is
         `min-w-0 flex-1`, so it fills whatever this wrapper is given. Full-bleed gives it
         a 48rem basis so it lands at that width instead of an even third of the row; the
         cap then hands the rest back to the side slots, which keeps it on the axis. -->
    <div class={['flex min-w-0 flex-1', fullBleed && 'max-w-3xl basis-3xl']}>
      {#if bareHeader}
        <!-- Hidden below 640px, where five labels do not fit beside the brand and the
             burger — and where the burger is a thumb away and lists every one of these
             with the same glyph and a full label. Shrinking them to bare icons would
             trade a legible row for five guesses. -->
        <nav aria-label="Site" class="hidden items-center gap-5 sm:flex lg:gap-6">
          <!-- Divides the nav from the wordmark, the same rule the search box draws
               between its Location scope and the field. Without it the first link sat
               one word-gap from "freehire" and read as part of it. -->
          <div aria-hidden="true" class="h-5 w-px shrink-0 bg-border"></div>
          {#each HEADER_LINKS as link (link.href)}
            {@const Icon = link.icon}
            <a
              href={resolve(link.href)}
              class="flex items-center gap-1.5 whitespace-nowrap text-sm text-muted-foreground transition-colors hover:text-foreground"
            >
              <Icon class="size-4 shrink-0" aria-hidden="true" />
              {link.label}
            </a>
          {/each}
        </nav>
      {:else}
        <HeaderSearch
          placeholder={listKind === 'companies' ? 'Search companies…' : 'Search jobs…'}
        />
      {/if}
    </div>

    <!-- One cluster, not two: the bell lives inside HeaderMenu beside the profile it
         notifies about, so this slot is just the menu. -->
    <div class={['flex shrink-0 items-center gap-1', fullBleed && 'flex-1 basis-0 justify-end']}>
      <HeaderMenu />
    </div>
  </div>
</header>
