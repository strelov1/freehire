<script lang="ts">
  import { LayoutGrid, Search, Tag } from '@lucide/svelte';
  import type { DropdownRow } from '#lib/dropdownRows.js';
  import { companyLogoUrl, sourceLogoUrl, SOURCE_LOGO_DOMAINS } from '#lib/logo.js';
  import { EntityLogo } from '#lib/ui/index.js';

  // What one row of HeaderSearch's suggestion listbox shows: a mark (logo or glyph) and
  // its text. The option element around it, with its keyboard and click handling, stays
  // in HeaderSearch.
  let { row }: { row: DropdownRow } = $props();
</script>

{#if row.kind === 'suggestion'}
  <!-- The glyph says which vocabulary the row comes from. A company gets its
       own mark, the same one the postings below carry, because a logo is
       what makes an employer scannable; everything else is a glyph. -->
  {#if row.suggestion.kind === 'company'}
    <EntityLogo
      name={row.suggestion.label}
      src={companyLogoUrl(row.suggestion.label) ?? undefined}
      shape="square"
      size="xs"
    />
  {:else if row.suggestion.kind === 'category'}
    <LayoutGrid class="size-4 shrink-0 text-muted-foreground" />
  {:else if row.suggestion.kind === 'title'}
    <Search class="size-4 shrink-0 text-muted-foreground" />
  {:else}
    <Tag class="size-4 shrink-0 text-muted-foreground" />
  {/if}
  <span class="min-w-0 flex-1 truncate">{row.suggestion.label}</span>
  {#if row.suggestion.count !== undefined}
    <span class="shrink-0 text-xs text-muted-foreground">{row.suggestion.count.toLocaleString()}</span>
  {/if}
{:else if row.kind === 'job'}
  <!-- Same mark the launcher dropdown renders, from the same resolver: the
       recognisable logo is what makes a row scannable at a glance. -->
  <EntityLogo
    name={row.job.company || 'Unknown company'}
    src={companyLogoUrl(row.job.company) ?? undefined}
    shape="square"
    size="xs"
  />
  <span class="min-w-0 flex-1">
    <span class="block truncate">{row.job.title}</span>
    <span class="block truncate text-xs text-muted-foreground">
      {row.job.company}{#if row.job.location}&nbsp;·&nbsp;{row.job.location}{/if}
    </span>
  </span>
{:else if row.kind === 'company'}
  <EntityLogo name={row.company.name} src={companyLogoUrl(row.company.name) ?? undefined} shape="square" size="xs" />
  <span class="min-w-0 flex-1 truncate">{row.company.name}</span>
  <span class="shrink-0 text-xs text-muted-foreground">
    {row.company.job_count}
    {row.company.job_count === 1 ? 'job' : 'jobs'}
  </span>
{:else if row.kind === 'operator'}
  {#if row.option.key === 'title'}
    <Search class="size-4 shrink-0 text-muted-foreground" />
  {:else}
    <EntityLogo
      name={row.option.label}
      src={(row.option.key === 'company'
        ? companyLogoUrl(row.option.label)
        : sourceLogoUrl(row.option.label, SOURCE_LOGO_DOMAINS[row.option.id])) ?? undefined}
      shape="square"
      size="xs"
    />
  {/if}
  <span class="min-w-0 flex-1 truncate">{row.option.label}</span>
  {#if row.option.count !== undefined}
    <span class="shrink-0 text-xs text-muted-foreground">{row.option.count.toLocaleString()}</span>
  {/if}
{:else if row.kind === 'text'}
  <Search class="size-4 shrink-0 text-muted-foreground" />
  <span class="min-w-0 flex-1 truncate text-muted-foreground">Search “{row.text}” as text</span>
{/if}
