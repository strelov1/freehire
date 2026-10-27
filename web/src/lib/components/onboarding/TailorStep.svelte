<script lang="ts">
  // The tour's CV tailoring step (Pro). The claims follow /features/tailor.
  import { resolve } from '$app/paths';
  import { ArrowRight, ArrowUpFromLine, FileDown, ScanSearch, ShieldCheck } from '@lucide/svelte';
  import TailorWorkspacePreview from '#lib/components/TailorWorkspacePreview.svelte';
  import FeatureGrid from './FeatureGrid.svelte';
  import { Badge } from '#lib/ui/index.js';
  import ProBar from './ProBar.svelte';
</script>

<h2 class="flex flex-wrap items-center gap-2 text-xl font-semibold tracking-tight text-balance">
  Tailoring CV <Badge variant="brand">Pro</Badge>
</h2>
<p class="mt-1 text-sm text-muted-foreground text-pretty">
  Your CV rewritten for one job, in the words the posting uses. Recruiters skim — this one gets read.
</p>

<div class="mt-6" aria-hidden="true" inert>
  <TailorWorkspacePreview />
</div>

<FeatureGrid
  items={[
    { icon: ScanSearch, title: 'Starts from the job', body: 'Reads the posting and your match.' },
    { icon: ArrowUpFromLine, title: 'Brings out buried experience', body: "Pulls forward what you've undersold." },
    { icon: ShieldCheck, title: 'Never makes things up', body: "It asks about what your history doesn't show." },
    { icon: FileDown, title: 'ATS-ready PDF and a cover letter', body: 'Both written for the same job.' },
  ]}
/>

<div class="mt-6 flex justify-end">
  <a
    href={resolve('/features/tailor')}
    target="_blank"
    class="inline-flex items-center gap-1 text-sm font-medium text-brand-strong hover:underline"
  >
    How CV tailoring works <ArrowRight class="size-3.5" aria-hidden="true" />
  </a>
</div>
<ProBar note="CV tailoring is part of Pro." />
