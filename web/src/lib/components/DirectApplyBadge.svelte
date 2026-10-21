<script lang="ts">
  import { BadgeCheck } from '@lucide/svelte';
  import { cn } from '$lib/ui';

  // Marks a vacancy its employer published on freehire and takes applications for only
  // here (jobview `direct_apply`). The brand tone is the point: it is the one thing on a
  // card a visitor cannot find on the employer's own careers page.
  let { class: className = '' }: { class?: string } = $props();
</script>

<span
  title="Published by the employer on freehire — apply here, your application goes straight to them"
  class={cn(
    'inline-flex items-center gap-1.5 rounded-md border border-brand/30 bg-brand-muted px-2 py-0.5 text-xs font-medium text-brand-strong',
    className,
  )}
>
  <BadgeCheck class="size-3.5 shrink-0" aria-hidden="true" />
  Only on freehire
</span>
