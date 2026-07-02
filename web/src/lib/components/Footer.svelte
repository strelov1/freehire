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
        { label: 'Jobs', href: resolve('/jobs') },
        { label: 'Companies', href: resolve('/companies') },
        { label: 'Collections', href: resolve('/collections') },
        { label: 'Recruiters', href: resolve('/recruiters') },
      ],
    },
    {
      title: 'Resources',
      links: [
        { label: 'CLI', href: resolve('/cli') },
        { label: 'API docs', href: resolve('/docs/api') },
      ],
    },
    {
      title: 'Company',
      links: [
        { label: 'For companies', href: resolve('/for-companies') },
        { label: 'Submit a job', href: resolve('/submit') },
      ],
    },
  ];

  // External profiles: open in a new tab, each rendered with its ProviderIcon brand
  // mark. github/telegram follow the text colour (so hover works); linkedin keeps
  // its brand blue by design.
  const socials = [
    { provider: 'github', label: 'GitHub', href: 'https://github.com/strelov1/freehire' },
    { provider: 'linkedin', label: 'LinkedIn', href: 'https://linkedin.com/company/freehire-dev/' },
    { provider: 'telegram', label: 'Telegram', href: 'https://t.me/freehiredev' },
  ];

  const year = new Date().getFullYear();
</script>

<footer class="border-t border-border">
  <div class="mx-auto max-w-6xl px-4 py-10 sm:py-12">
    <div class="grid grid-cols-2 gap-8 sm:grid-cols-4 sm:gap-6">
      <!-- Brand block: full width on mobile, one column on wide viewports. -->
      <div class="col-span-2 sm:col-span-1 sm:pr-4">
        <a
          href={resolve('/')}
          class="flex items-center gap-2 text-sm font-semibold tracking-tight"
        >
          <!-- Same mark as the header; tracks the theme via currentColor. -->
          <svg
            viewBox="0 0 512 512"
            class="size-5 shrink-0"
            fill="currentColor"
            aria-hidden="true"
            xmlns="http://www.w3.org/2000/svg"
          >
            <path
              fill-rule="evenodd"
              clip-rule="evenodd"
              d="M256 56C366.457 56 456 145.543 456 256C456 366.457 366.457 456 256 456C145.543 456 56 366.457 56 256C56 145.543 145.543 56 256 56ZM256 166L346 256L256 346L166 256L256 166Z"
            />
          </svg>
          <span>FreeHire</span>
        </a>
        <p class="mt-3 max-w-xs text-sm text-muted-foreground">
          Free, open-source IT job aggregator.
        </p>
        <div class="mt-4 flex items-center gap-3">
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

  <!-- Bottom bar: copyright + open-source note, split off by a thin border. -->
  <div class="border-t border-border">
    <div
      class="mx-auto flex max-w-6xl flex-col gap-1 px-4 py-4 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between"
    >
      <p>© {year} FreeHire</p>
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
