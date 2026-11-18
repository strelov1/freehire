<script lang="ts">
  import { CheckCircle2 } from '@lucide/svelte';
  import { Chip } from '#lib/ui/index.js';
  import AppliedCompanyBadge from './AppliedCompanyBadge.svelte';

  // The job page's title, with the reader's own applied state beside it.
  let {
    title,
    lang,
    applied,
    appliedToCompany,
  }: { title: string; lang: string | undefined; applied: boolean; appliedToCompany: boolean } = $props();
</script>

<div class="flex flex-wrap items-center gap-2.5">
  <h1 class="text-2xl font-semibold tracking-tight" {lang}>{title}</h1>
  <!-- Applied stays with the title: it is a fact about THIS READER, not about the
       posting, so it does not belong on the provenance line above. -->
  {#if applied}
    <Chip variant="brand" class="gap-1.5 border-brand/30 font-semibold">
      <CheckCircle2 class="size-3.5" aria-hidden="true" /> Applied
    </Chip>
  {:else if appliedToCompany}
    <AppliedCompanyBadge />
  {/if}
</div>
