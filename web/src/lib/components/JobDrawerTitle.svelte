<script lang="ts">
  import { resolve } from '$app/paths';
  import { ExternalLink, X } from '@lucide/svelte';
  import { EntityLogo } from '#lib/ui/index.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { companyLogoUrl } from '#lib/logo.js';
  import { messages } from './JobDrawer.messages';

  // The job drawer's heading: the employer, a link to the posting, the close control,
  // the title and its tags.
  let {
    company,
    title,
    tags,
    jobSlug,
    onClose,
  }: { company: string; title: string; tags: string[]; jobSlug: string | undefined; onClose: () => void } = $props();

  const s = $derived(t(messages, locale()));
</script>

<div class="flex items-center gap-3">
  <EntityLogo
    name={company || s.unknownCompany}
    src={companyLogoUrl(company) ?? undefined}
    shape="square"
    size="sm"
    class="shrink-0"
  />
  <p class="min-w-0 flex-1 break-words text-sm font-medium text-muted-foreground">
    {company || s.unknownCompany}
  </p>
  {#if jobSlug}
    <a
      href={resolve('/jobs/[slug]', { slug: jobSlug })}
      target="_blank"
      rel="noopener noreferrer"
      class="inline-flex min-h-11 shrink-0 items-center gap-1.5 rounded px-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring sm:min-h-8 sm:text-sm"
    >
      <ExternalLink class="size-3.5" aria-hidden="true" />
      {s.viewJob}
    </a>
  {/if}
  <button
    type="button"
    onclick={onClose}
    class="-mr-1.5 flex size-11 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground sm:size-8"
    aria-label={s.close}
  >
    <X class="size-5" />
  </button>
</div>

<h2 class="text-balance text-xl font-semibold leading-tight tracking-tight">{title}</h2>

{#if tags.length}
  <p class="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
    {#each tags as tag (tag)}
      <span>{tag}</span>
    {/each}
  </p>
{/if}
