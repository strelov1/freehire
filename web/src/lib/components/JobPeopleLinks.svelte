<script lang="ts">
  import { resolve } from '$app/paths';
  import { SquarePen } from '@lucide/svelte';
  import { ProviderIcon } from '#lib/ui/index.js';

  // The tools beside the job page's tabs for reaching people at the employer: drafting a
  // referral message (not on a closed posting; `loading` while its form loads) and finding
  // people at `company` on LinkedIn through the posting's own people route (`slug`).
  let {
    slug,
    company,
    closed,
    loading,
    ondraft,
  }: { slug: string; company: string; closed: boolean; loading: boolean; ondraft: () => void } = $props();
</script>

<div class="flex shrink-0 items-center self-stretch border-b border-border lg:gap-x-4 lg:pl-3">
  {#if !closed}
    <button
      type="button"
      disabled={loading}
      onclick={ondraft}
      aria-label="Draft referral message"
      title="Draft a referral message"
      class="inline-flex min-h-11 min-w-11 shrink-0 items-center justify-center gap-1.5 text-sm text-muted-foreground hover:text-foreground disabled:opacity-50 lg:min-w-0"
    >
      <SquarePen class="size-4 shrink-0 lg:size-3.5" aria-hidden="true" />
      <span class="hidden lg:inline">Draft referral</span>
    </button>
  {/if}
  <a
    href={resolve('/jobs/[slug]/people', { slug })}
    target="_blank"
    rel="nofollow noopener noreferrer"
    aria-label="Find people at {company} on LinkedIn"
    title="Find people at {company} on LinkedIn"
    class="inline-flex min-h-11 min-w-11 shrink-0 items-center justify-center gap-1.5 text-sm text-muted-foreground hover:text-foreground lg:min-w-0"
  >
    <span class="hidden lg:inline">Find people</span>
    <ProviderIcon provider="linkedin" class="size-4 shrink-0 lg:size-3.5" />
  </a>
</div>
