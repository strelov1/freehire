<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { afterNavigate, replaceState } from '$app/navigation';
  import { authDialog, openAuthDialog, closeAuthDialog } from '$lib/auth-dialog.svelte';
  import AuthDialog from './AuthDialog.svelte';
  import HeaderSearch from './HeaderSearch.svelte';
  import HeaderListSearch from './HeaderListSearch.svelte';
  import HeaderMenu from './HeaderMenu.svelte';
  import BrandMark from './BrandMark.svelte';

  // The header is three slots — logo | search | menu — identical on every
  // viewport. Nav links, the account items, the theme toggle, and the auth
  // action all live in HeaderMenu.
  //
  // The middle slot is one text field that adapts to context: on the list pages
  // (the homepage feed `/`, /companies, and a company's own /companies/:slug jobs
  // list) it IS that page's filter (HeaderListSearch drives the list, so there's no
  // duplicate box); everywhere else it's the global launcher with the instant
  // dropdown (HeaderSearch). A company detail page is a jobs list scoped to that
  // company, so the header search filters its postings — hence 'company' shares the
  // jobs proxy.
  const listKind = $derived(
    page.url.pathname === '/'
      ? 'jobs'
      : page.url.pathname === '/companies'
        ? 'companies'
        : /^\/companies\/[^/]+$/.test(page.url.pathname)
          ? 'company'
          : null,
  );

  // The auth dialog lives at the layout level but its open state is a shared
  // singleton (see auth-dialog.svelte), so deep components — like a job's Save
  // button — can prompt sign-in through the same dialog this header renders.

  // Surface auth prompts carried in the URL on the client, then clean it.
  // ?auth_error: a failed OAuth callback. ?auth=required: a guarded page (e.g.
  // /my/tracking, /jobs/swipe) bounced a signed-out visitor here to sign in. Runs in
  // afterNavigate — not onMount — because this header lives in the persistent root
  // layout: a guard that redirects here via client-side navigation never remounts
  // it, so onMount would fire only on a cold load and miss the in-app bounce.
  // afterNavigate covers both the initial load and every later navigation, and
  // stays off the SSR path. The replaceState clean-up below removes the params, so
  // the immediate re-run sees none and no loop forms.
  // Only accept a same-origin rooted path as the post-login redirect — never a
  // scheme-relative "//host" or absolute URL — mirroring the backend's
  // SafeReturnPath, so a crafted link can't bounce the user off-site.
  function safeRedirect(raw: string | null): string | null {
    if (!raw || !raw.startsWith('/') || raw.startsWith('//')) return null;
    return raw;
  }

  afterNavigate(() => {
    const params = page.url.searchParams;
    if (params.has('auth_error')) {
      // A real failure: seed the dialog's error banner.
      openAuthDialog('login', 'Sign-in failed. Please try again.');
    } else if (params.get('auth') === 'required') {
      // Just a sign-in gate, not an error — open the dialog with no error banner.
      // ?redirect (set by a guarded page) is the deep link to return to after
      // sign-in; stash it before the URL is cleaned below.
      openAuthDialog('login', null, safeRedirect(params.get('redirect')));
    } else {
      return;
    }
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- shallow same-page URL clean-up to the current pathname; nothing to resolve
    replaceState(page.url.pathname, {});
  });
</script>

<!-- Solid (not backdrop-blur) on purpose: a backdrop-filter here would become the
     containing block for the mobile menu's `position: fixed` full-screen drawer,
     pinning it to the header instead of the viewport and breaking it. -->
<header class="sticky top-0 z-40 border-b border-border bg-background">
  <div class="mx-auto flex h-14 max-w-6xl items-center gap-3 px-4 sm:gap-4">
    <a
      href={resolve('/')}
      class="flex shrink-0 items-center gap-2 text-sm font-semibold tracking-tight"
    >
      <BrandMark />
      <span class="hidden sm:inline">freehire</span>
    </a>

    {#if listKind === 'jobs' || listKind === 'company'}
      <HeaderListSearch placeholder="Search jobs…" />
    {:else if listKind === 'companies'}
      <HeaderListSearch placeholder="Search companies…" />
    {:else}
      <HeaderSearch />
    {/if}

    <HeaderMenu />
  </div>
</header>

{#if authDialog.open}
  <AuthDialog bind:mode={authDialog.mode} initialError={authDialog.error} onClose={closeAuthDialog} />
{/if}
