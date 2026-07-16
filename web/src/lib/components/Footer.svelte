<script lang="ts">
  import { resolve } from '$app/paths';
  import ProviderIcon from './ProviderIcon.svelte';

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
      title: 'Resources',
      links: [
        { label: 'Blog', href: resolve('/blog') },
        { label: 'Insights', href: resolve('/insights') },
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
        { label: 'Submit a job', href: resolve('/submit') },
        { label: 'Privacy', href: resolve('/privacy') },
      ],
    },
  ];

  // External profiles: open in a new tab, each rendered with its ProviderIcon brand
  // mark. All three follow the muted text colour (so they match and hover works).
  const socials = [
    { provider: 'github', label: 'GitHub', href: 'https://github.com/strelov1/freehire' },
    { provider: 'linkedin', label: 'LinkedIn', href: 'https://linkedin.com/company/freehire-dev/' },
    { provider: 'telegram', label: 'Telegram', href: 'https://t.me/freehiredev' },
  ];

  const year = new Date().getFullYear();
</script>

<footer class="border-t border-border">
  <div class="mx-auto max-w-6xl px-4 py-8 sm:py-12">
    <div class="grid grid-cols-3 gap-x-6 gap-y-7 sm:gap-6">
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
  </div>

  <!-- Bottom bar: copyright + social links on the left, open-source note on the
       right, split off by a thin border. -->
  <div class="border-t border-border">
    <div
      class="mx-auto flex max-w-6xl flex-col gap-3 px-4 py-4 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between sm:gap-1"
    >
      <div class="flex items-center gap-4">
        <p>© {year}</p>
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
