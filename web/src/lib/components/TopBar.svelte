<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { toPath } from '#lib/pathname.js';
  import { afterNavigate, goto } from '$app/navigation';
  import { signinUrl } from '#lib/signin.js';
  import HeaderSearch from './HeaderSearch.svelte';
  import HeaderMenu from './HeaderMenu.svelte';
  import BrandMark from './BrandMark.svelte';
  import { isApiReferenceRoute, isWideHeaderRoute } from '#lib/shellLayout.js';
  import { HEADER_LINKS } from '#lib/siteNav.js';
  import { siteHeader } from '#lib/siteHeader.svelte.js';

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
  // How the box words itself is all the page decides — it asks nothing else. This used
  // to select between two components that shared everything but that one behaviour and
  // drifted apart in the parts they shared.
  //
  // Keep header hints static; animated role examples belong only to the homepage hero.
  const searchWording = $derived(
    page.url.pathname === '/companies'
      ? { placeholder: 'Search companies…', label: 'Search companies' }
      : { placeholder: 'Search jobs…', label: 'Search jobs' },
  );

  // The homepage is the one exception: its whole content is a large centred copy of
  // this same box, so the header renders none. Two identical fields on one screen make
  // the visitor choose between them for no reason, and the hero is the focused one.
  // The brand and the menu stay — sign-in and notifications live in the menu's own
  // control strip, and both have to remain reachable.
  const bareHeader = $derived(page.url.pathname === '/');

  // The API reference is the one page below that has its own search already: Scalar's
  // sidebar answers Cmd/Ctrl+K over the spec, and it is already mounted the moment the
  // reference hydrates. HeaderSearch binds those same global hotkeys for job search
  // (see its own `onWindowKeydown`), so leaving it mounted here would not just be a
  // search box with nothing to search — it would fight Scalar's for the very keys that
  // are supposed to open it. The site nav fills the slot instead, the same as the
  // homepage — see showSiteNav below, which ties the two together.
  const docsApiHeader = $derived(isApiReferenceRoute(page.url.pathname));

  // Wherever the middle slot has no search box, the row has the room for both the nav
  // AND the wordmark on a phone (see the wordmark's own comment below) — today that is
  // bareHeader and docsApiHeader. Named once so the wordmark's visibility and the nav's
  // own render condition can never drift apart the way they did before this existed: the
  // nav gained docsApiHeader in one change, the wordmark's identical "is there room"
  // condition did not, and phones showed the nav beside a bare logo glyph with no name.
  const showSiteNav = $derived(bareHeader || docsApiHeader);

  /** How many of HEADER_LINKS the bare header carries below `lg`, taken from the FRONT of
   *  that list — so its order is the contract, and reordering it changes what a narrow
   *  screen gets. Today that is Jobs and Companies.
   *
   *  Two, because that is what fits at the narrow end (33px to spare at 320px, labels
   *  only — see the nav below). `lg` rather than a width nearer the 800px where the rest
   *  do begin to fit, because Tailwind's own breakpoints are what the rest of this header
   *  is written in and one more arbitrary query is a second answer to the same question.
   *  Under `lg` the burger lists them all anyway, which is where they were until now.
   *
   *  Deliberately a count and not a slice of named links: the row IS HEADER_LINKS' own
   *  order, so a destination added to that list or moved within it changes what a narrow
   *  screen gets without anything here needing to know. */
  const LINKS_BELOW_LG = 2;

  // On the full-viewport surfaces (the agent, the tailor workspace) the page below runs
  // edge to edge under its own icon rail, and on the API reference the columns below
  // (sidebar, content, request/response examples) are just as wide — so on all of them
  // the header drops the centered `max-w-6xl` and does the same: brand hard left, menu
  // hard right. The search keeps a readable width and centers itself in the gap rather
  // than stretching across the monitor.
  const wideHeader = $derived(isWideHeaderRoute(page.url.pathname));

  // The header slides away while the visitor scrolls down and comes back on any scroll
  // up, the way Safari's bars do: reading a long list gets the full screen, and the
  // header is one flick away rather than at the top of the page. Near the top it always
  // shows, and it comes back whenever focus enters it (a keyboard user tabbing to it).
  // The state lives on <html> as `data-header-hidden` so the sticky rails pinned under
  // the header (`top-header`, `top-header-air` in app.css) follow it from one variable.
  // `top` rather than a transform: a transform would become the containing block for
  // the mobile menu's `position: fixed` drawer, the same trap as backdrop-blur below.
  let lastScrollY = 0;
  let headerEl: HTMLElement | undefined = $state();
  const SCROLL_SLOP = 6; // ignore trackpad jitter so the bar does not flicker
  function onWindowScroll() {
    // Clamped: Safari's rubber-band reports scrollY past either edge, and settling back would read as a scroll.
    const y = Math.min(Math.max(window.scrollY, 0), document.documentElement.scrollHeight - window.innerHeight);
    const delta = y - lastScrollY;
    // The header's own height (taller under a larger root font): it never slides away over nothing.
    const hideAfter = headerEl?.offsetHeight ?? 57;
    if (y < hideAfter) siteHeader.hidden = false;
    // Never slide away a control that holds focus: PageDown from a focused link would hide it.
    else if (delta > SCROLL_SLOP) siteHeader.hidden = !headerEl?.contains(document.activeElement);
    else if (delta < -SCROLL_SLOP) siteHeader.hidden = false;
    if (Math.abs(delta) > SCROLL_SLOP || y < hideAfter) lastScrollY = y;
  }
  $effect(() => {
    document.documentElement.toggleAttribute('data-header-hidden', siteHeader.hidden);
  });

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
  // It also brings back a header the previous page scrolled away.
  afterNavigate(({ shallow }) => {
    siteHeader.hidden = false;
    if (shallow) return; // SvelteKit 3 fires this for shallow routing too
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
<svelte:window onscroll={onWindowScroll} />
<header
  bind:this={headerEl}
  class="top-site-header sticky z-40 border-b border-border bg-background"
  onfocusin={() => (siteHeader.hidden = false)}
>
  <div class={['mx-auto flex h-14 items-center gap-3 px-4 sm:gap-4', wideHeader ? 'max-w-none' : 'max-w-6xl']}>
    <!-- Wide header only: the two side slots grow from a zero basis, so the free space
         splits evenly between them and the search sits on the container's axis — the
         menu cluster is wider than the brand, so centering the middle slot on its own
         leftover space would push it visibly off-centre. `basis-0` also keeps them from
         shrinking (shrink scales the basis), so the narrow layout is untouched. -->
    <div class={['flex shrink-0 items-center', wideHeader && 'flex-1 basis-0']}>
      <a href={resolve('/')} aria-label="freehire" class="flex items-center gap-2 text-sm font-semibold tracking-tight">
        <BrandMark />
        <!-- Dropped on a phone to leave the middle slot its width for the search box —
             except wherever showSiteNav puts no box there (the homepage's own hero is the
             box; the API reference's is Scalar's), which has the room. What the row spends
             that room on instead is the nav below, and the two budgets are the same 358px:
             they are set together. -->
        <span class={showSiteNav ? 'inline' : 'hidden sm:inline'} aria-hidden="true">freehire</span>
      </a>
    </div>

    <!-- The slot, not the search component, owns the middle width: each search root is
         `min-w-0 flex-1`, so it fills whatever this wrapper is given. The wide header gives it
         a 48rem basis so it lands at that width instead of an even third of the row; the
         cap then hands the rest back to the side slots, which keeps it on the axis. -->
    <div class={['flex min-w-0 flex-1', wideHeader && 'max-w-3xl basis-3xl']}>
      {#if showSiteNav}
        <!-- All five do not fit beside the brand and the burger on a phone, but the row
             they share is otherwise EMPTY on these routes — the homepage's search box is
             its hero, not this slot, and the API reference's own search is Scalar's — so
             the front of the list rides along and the rest wait for `lg` (see
             LINKS_BELOW_LG). The cut is by width, not by rank; the burger a thumb away
             still lists all five with the same glyph and a full label, which is also why
             bare icons here would trade a legible row for guesses. -->
        <nav aria-label="Site" class="flex items-center gap-3 sm:gap-5 lg:gap-3 xl:gap-6">
          <!-- Divides the nav from the wordmark, the same rule the search box draws
               between its Location scope and the field. Without it the first link sat
               one word-gap from "freehire" and read as part of it. -->
          <div aria-hidden="true" class="h-5 w-px shrink-0 bg-border"></div>
          <!-- Labels only, no glyphs: a row of icon+word pairs reads as one busy strip,
               and the word already names the destination. The burger keeps the glyphs. -->
          {#each HEADER_LINKS as link, i (link.href)}
            <a
              href={resolve(toPath(link.href))}
              class={[
                'flex items-center whitespace-nowrap text-sm text-muted-foreground transition-colors hover:text-foreground',
                i >= LINKS_BELOW_LG && 'hidden lg:flex',
              ]}
            >
              {link.label}
            </a>
          {/each}
        </nav>
      {:else}
        <HeaderSearch {...searchWording} />
      {/if}
    </div>

    <!-- One cluster, not two: the bell lives inside HeaderMenu beside the profile it
         notifies about, so this slot is just the menu. -->
    <div class={['flex shrink-0 items-center gap-1', wideHeader && 'flex-1 basis-0 justify-end']}>
      <HeaderMenu />
    </div>
  </div>
</header>
