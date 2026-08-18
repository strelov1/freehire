<script lang="ts">
  import { resolve } from '$app/paths';
  import { popularCollectionLinks } from '$lib/collections';
  import { reopen } from '$lib/consent.svelte';
  import { ProviderIcon } from '$lib/ui';

  // Grouped navigation over existing routes only — kept deliberately small so the
  // footer stays uncluttered. Internal links go through resolve() (base-path safe),
  // mirroring the header.
  const groups = [
    {
      title: 'Product',
      links: [
        { label: 'Jobs', href: resolve('/') },
        { label: 'Companies', href: resolve('/companies') },
        { label: 'Collections', href: resolve('/collections') },
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
        { label: 'Referrals', href: resolve('/features/referrals') },
        { label: 'Ghost jobs', href: resolve('/features/ghost-jobs') },
      ],
    },
    {
      title: 'Resources',
      links: [
        { label: 'Blog', href: resolve('/blog') },
        { label: 'Insights', href: resolve('/insights') },
        { label: 'Hiring signal', href: resolve('/insights/companies') },
        { label: 'Trends', href: resolve('/trends') },
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
        { label: 'Submit a job', href: resolve('/submit') },
        { label: 'Privacy', href: resolve('/privacy') },
        { label: 'Terms', href: resolve('/terms') },
      ],
    },
  ];

  // External profiles: open in a new tab, each rendered with its ProviderIcon brand
  // mark. All three follow the muted text colour (so they match and hover works).
  const socials = [
    { provider: 'github', label: 'GitHub', href: 'https://github.com/strelov1/freehire' },
    { provider: 'linkedin', label: 'LinkedIn', href: 'https://linkedin.com/company/freehire-dev/' },
    { provider: 'telegram', label: 'Telegram', href: 'https://t.me/freehiredev' },
    { provider: 'discord', label: 'Discord', href: 'https://discord.gg/aAXS2rghW' },
  ];

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
</script>

<footer class="border-t border-border">
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

    <div class="mt-8">
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
    </div>
  </div>

  <!-- Bottom bar: copyright + social links on the left, open-source note on the
       right, split off by a thin border. -->
  <div class="border-t border-border">
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
          {#each socials as social (social.provider)}
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
