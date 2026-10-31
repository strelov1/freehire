<script lang="ts">
  import { companyLogoUrl } from '#lib/logo.js';
  import { EntityLogo, cn } from '#lib/ui/index.js';

  // A company's mark as the buddies space draws it: round, on white, big enough to read.
  // The logo proxy is asked by name; a miss falls back to the name's initials.
  let { name, size = 'md', class: className }: { name: string; size?: 'sm' | 'md' | 'lg'; class?: string } = $props();
</script>

<EntityLogo {name} src={companyLogoUrl(name) ?? undefined} {size} class={cn('shrink-0 bg-white ring-1 ring-border', className)} />
