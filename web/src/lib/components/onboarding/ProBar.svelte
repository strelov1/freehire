<script lang="ts">
  // What is paid on this step, and the way to the plans.
  import { resolve } from '$app/paths';
  import { Button } from '#lib/ui/index.js';

  let { note }: { note: string } = $props();
</script>

<div class="mt-7 flex flex-wrap items-center gap-x-4 gap-y-3 rounded-xl border border-brand/30 bg-brand-muted/40 p-4">
  <p class="min-w-0 flex-1 text-sm text-pretty">{note}</p>
  <Button variant="primary" size="sm" href={resolve('/pricing')} target="_blank">See Pro plans</Button>
</div>
