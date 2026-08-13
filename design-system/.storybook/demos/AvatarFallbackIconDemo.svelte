<script lang="ts">
  // fallbackIcon is a Snippet, so a story cannot hand it through plain args — this demo
  // shows the entity-tile case CompanyLogo used it for: no name at all, just a mark.
  import Avatar from '../../src/avatar.svelte';
  import { Globe } from '@lucide/svelte';

  let { size = 'md' }: { size?: 'sm' | 'md' | 'lg' } = $props();
</script>

<Avatar {size} shape="square">
  {#snippet fallbackIcon()}
    <Globe class="size-1/2" />
  {/snippet}
</Avatar>
