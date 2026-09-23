<script lang="ts">
  import { resolve } from '$app/paths';
  import { popularCollectionLinks } from '$lib/collections';
  import { reopen } from '$lib/consent.svelte';
  import { EXTENSION_STORE_URL } from '$lib/extensionLinks';
  import { SOCIAL_LINKS } from '$lib/socialLinks';
  import { ProviderIcon } from '$lib/ui';

  // Grouped navigation over existing routes only — kept deliberately small so the
  // footer stays uncluttered. Internal links go through resolve() (base-path safe),
  // mirroring the header.
  const groups = [
    {
      title: 'Product',
      links: [
        { label: 'Jobs', href: resolve('/jobs') },
        { label: 'Companies', href: resolve('/companies') },
        { label: 'Collections', href: resolve('/collections') },
        { label: 'Talent Network', href: resolve('/talent') },
        { label: 'Jobs by role', href: resolve('/roles') },
        // The glossary's only link from the app — the chip's reveal opens on
        // interaction and the sitemap is for crawlers, so this is the one path a
        // reader browsing the site can follow to it.
        { label: 'Skills glossary', href: resolve('/skills') },
        { label: 'Recruiters', href: resolve('/recruiters') },
      ],
    },
    {
      title: 'Features',
      links: [
        { label: 'Advanced search', href: resolve('/features/advanced-search') },
        { label: 'Browser extension', href: resolve('/features/extension') },
        { label: 'Inbox', href: resolve('/features/inbox') },
        { label: 'CV tailoring', href: resolve('/features/tailor') },
        { label: 'Application tracking', href: resolve('/features/tracking') },
        { label: 'Notifications', href: resolve('/features/notifications') },
        { label: 'Referrals', href: resolve('/features/referrals') },
        { label: 'Ghost jobs', href: resolve('/features/ghost-jobs') },
      ],
    },
    {
      title: 'Resources',
      links: [
        { label: 'How it works', href: resolve('/how-it-works') },
        { label: 'Discussions', href: resolve('/discussions') },
        { label: 'Blog', href: resolve('/blog') },
        { label: 'Insights', href: resolve('/insights') },
        { label: 'Hiring signal', href: resolve('/insights/companies') },
        { label: 'Trends', href: resolve('/trends') },
        { label: 'For AI agents', href: resolve('/agents') },
        { label: 'CLI', href: resolve('/cli') },
        { label: 'ChatGPT', href: resolve('/chatgpt') },
        { label: 'API docs', href: resolve('/docs/api') },
      ],
    },
    {
      title: 'Company',
      links: [
        { label: 'About', href: resolve('/about') },
        { label: 'Open', href: resolve('/open') },
        { label: 'For companies', href: resolve('/for-companies') },
        { label: 'Contribute', href: resolve('/contribute') },
        // Next to Contribute rather than under Resources: the two are one thought —
        // here is how to help, and here is everyone who did.
        { label: 'Contributors', href: resolve('/contributors') },
        { label: 'Submit a job', href: resolve('/submit') },
        // Next to Status rather than under Resources: the two answer neighbouring
        // questions — where the jobs come from, and whether we are still reading them.
        { label: 'Sources', href: resolve('/sources') },
        { label: 'Status', href: resolve('/status') },
        { label: 'Support', href: resolve('/support') },
        { label: 'Privacy', href: resolve('/privacy') },
        { label: 'Terms', href: resolve('/terms') },
      ],
    },
  ];

  // External profiles: open in a new tab, each rendered with its ProviderIcon brand
  // mark. All follow the muted text colour (so they match and hover works). Shared
  // with the /signin brand panel — see $lib/socialLinks.

  // A strip below the four groups rather than a fifth column: the grid is
  // sm:grid-cols-4, and ten links would not fit one anyway. Kept out of `groups`
  // because these are collection landing pages, not site navigation — and because
  // it is the one place every page links them from (see popularCollectionLinks).
  const popular = popularCollectionLinks();

  const year = new Date().getFullYear();

  // Product Hunt "featured" badge. Two embed URLs — one per theme — swapped by the
  // `dark` class variant rather than by reading themeStore, so the right one is
  // already in the SSR markup (the anti-FOUC script in app.html sets `.dark` before
  // paint). Both URLs are Product Hunt's own, copied verbatim including the `t=`
  // cache-buster it stamps per variant.
  const productHunt = {
    href: 'https://www.producthunt.com/products/freehire?embed=true&utm_source=badge-featured&utm_medium=badge&utm_campaign=badge-freehire',
    alt: 'freehire - The open-source job search that covers every board | Product Hunt',
    light:
      'https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=1196233&theme=light&t=1785605037608',
    dark: 'https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=1196233&theme=dark&t=1785605357228',
  };

  // Where the product can be installed, beside the Product Hunt badge rather than in
  // a link column: these are downloads, not navigation, and a store button among a
  // list of page links reads as neither.
  //
  // Drawn from the design system's own marks and tokens instead of each store's
  // official badge artwork — the two vendors' badges disagree about height, corner
  // radius and dark-mode treatment, so side by side they look like two pasted
  // screenshots. One shape in the site's own colours matches the footer around it
  // and follows the theme without a second asset per mode; ProviderIcon already
  // ships both marks, so this needs no image at all.
  //
  // The App Store URL carries NO country segment on purpose. The listing's share link
  // is `/br/app/...`, which pins every visitor to the Brazilian storefront; without one
  // Apple redirects each visitor to their own (measured: the bare form answers 301 to
  // `/us/app/...`, the `/br/` form answers 200).
  //
  // The extension's URL is not spelled here at all — see $lib/extensionLinks, which the
  // extension landing and its JSON-LD read too. The App Store one IS spelled here, and
  // the asymmetry is deliberate only while this is its single consumer: a mobile landing
  // page or a MobileApplication JSON-LD is the second, and that is when it earns an
  // appLinks.ts beside extensionLinks.ts. Note for whoever does it — Footer.test.ts pins
  // the literal to THIS file, so the move is two edits, not one.
  const stores = [
    {
      provider: 'apple',
      kicker: 'Download on the',
      name: 'App Store',
      href: 'https://apps.apple.com/app/freehire-open-job-search/id6801885119',
    },
    {
      provider: 'chrome',
      kicker: 'Available in the',
      name: 'Chrome Web Store',
      href: EXTENSION_STORE_URL,
    },
  ];

  // Compact: the account shell (/my/*) is an app-like surface with its own sidebar
  // nav, so the four link columns, the popular-collections strip and the Product
  // Hunt badge below are marketing chrome that doesn't belong there — only the
  // bottom bar (copyright, cookie settings, social links, open-source note) still
  // applies. Full-bleed account routes (/my/assistant/*, /tailor/*) render no
  // footer at all, compact or otherwise — see routes/+layout.svelte's hideFooter.
  let { compact = false }: { compact?: boolean } = $props();
</script>

<footer class="border-t border-border">
  {#if !compact}
    <div class="mx-auto max-w-6xl px-4 py-8 sm:py-12">
      <div class="grid grid-cols-2 gap-x-6 gap-y-7 sm:grid-cols-4 sm:gap-6">
        <!-- Navigation groups. Each is a named landmark (aria-label) so screen readers
             get a title without adding headings to the page outline. -->
        {#each groups as group (group.title)}
          <nav class="flex flex-col gap-3" aria-label={group.title}>
            <p class="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              {group.title}
            </p>
            <ul class="flex flex-col gap-2">
              {#each group.links as link (link.href)}
                <li>
                  <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- internal route already passed through resolve() when building `groups`; the linter can't trace it via the variable -->
                  <a href={link.href}
                    class="text-sm text-muted-foreground transition-colors hover:text-foreground"
                  >
                    {link.label}
                  </a>
                </li>
              {/each}
            </ul>
          </nav>
        {/each}
      </div>

      <!-- Popular collections. Real <a href> in the server-rendered HTML: crawlers
           discover links by parsing markup, and these landing pages had none from the
           homepage at all. -->
      <nav class="mt-8 border-t border-border pt-6" aria-label="Popular collections">
        <p class="text-xs font-medium uppercase tracking-wider text-muted-foreground">Popular</p>
        <ul class="mt-3 flex flex-wrap gap-x-4 gap-y-2">
          {#each popular as collection (collection.slug)}
            <li>
              <a href={resolve('/collections/[slug]', { slug: collection.slug })}
                class="text-sm text-muted-foreground transition-colors hover:text-foreground"
              >
                {collection.title}
              </a>
            </li>
          {/each}
        </ul>
      </nav>

      <!-- The badge and the two store buttons share one row, centred rather than
           stretched: the badge is a fixed 54px image, the buttons sit on the spacing
           scale at h-14 (56px), and 1px above and below is invisible. Do NOT "fix" that
           to h-[54px] — check:tokens refuses an arbitrary value here, which is the whole
           reason these are 56px. Wrapping, so a narrow viewport stacks them rather than
           shrinking any. -->
      <div class="mt-8 flex flex-wrap items-center gap-3">
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external Product Hunt page opened in a new tab; not an internal route -->
        <a href={productHunt.href} target="_blank" rel="noopener noreferrer" class="inline-block">
          <img
            src={productHunt.light}
            alt={productHunt.alt}
            width="250"
            height="54"
            loading="lazy"
            class="dark:hidden"
          />
          <img
            src={productHunt.dark}
            alt={productHunt.alt}
            width="250"
            height="54"
            loading="lazy"
            class="hidden dark:block"
          />
        </a>

        {#each stores as store (store.href)}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external store listing opened in a new tab; not an internal route -->
          <a href={store.href}
            target="_blank"
            rel="noopener noreferrer"
            class="inline-flex h-14 items-center gap-3 rounded-lg border border-border px-4 text-foreground transition-colors hover:bg-muted"
          >
            <ProviderIcon provider={store.provider} class="size-7 shrink-0" />
            <span class="flex flex-col leading-tight">
              <span class="text-xs uppercase tracking-wider text-muted-foreground">
                {store.kicker}
              </span>
              <span class="text-sm font-semibold">{store.name}</span>
            </span>
          </a>
        {/each}
      </div>
    </div>
  {/if}

  <!-- Bottom bar: copyright + social links on the left, open-source note on the
       right. Split off by its own top border only when it follows the link
       groups above — compact, the footer's own top border already does that job. -->
  <div class={compact ? undefined : 'border-t border-border'}>
    <div
      class="mx-auto flex max-w-6xl flex-col gap-3 px-4 py-4 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between sm:gap-1"
    >
      <div class="flex items-center gap-4">
        <p>© {year}</p>
        <!-- Re-opens the consent banner so a prior cookie choice can be changed —
             withdrawal as easy as granting (GDPR). -->
        <button
          type="button"
          onclick={reopen}
          class="text-muted-foreground transition-colors hover:text-foreground"
        >
          Cookie settings
        </button>
        <div class="flex items-center gap-3">
          {#each SOCIAL_LINKS as social (social.provider)}
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external profile URL opened in a new tab; not an internal route -->
            <a href={social.href}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={social.label}
              class="text-muted-foreground transition-colors hover:text-foreground"
            >
              <ProviderIcon provider={social.provider} />
            </a>
          {/each}
        </div>
      </div>
      <p>
        Free &amp; open-source.
        <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external repository URL opened in a new tab; not an internal route -->
        <a href="https://github.com/strelov1/freehire"
          target="_blank"
          rel="noopener noreferrer"
          class="font-medium text-foreground transition-colors hover:text-muted-foreground"
        >
          View source on GitHub
        </a>.
      </p>
    </div>
  </div>
</footer>
