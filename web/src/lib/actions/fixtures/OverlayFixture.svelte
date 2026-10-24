<script lang="ts">
  import { focusTrap } from '../focusTrap.js';
  import { menuNav } from '../menuNav.js';

  // A trigger plus an overlay rendered only while open — the shape every real host
  // has — so the specs exercise the attachments' cleanup the way Svelte runs it.
  let { kind }: { kind: 'trap' | 'menu' } = $props();
  let open = $state(false);
</script>

<button type="button" onclick={() => (open = !open)}>Open</button>
<button type="button" onclick={() => (open = false)}>Close from outside</button>
{#if open}
  {#if kind === 'trap'}
    <div role="dialog" aria-modal="true" aria-label="Overlay" {@attach focusTrap()}>
      <button type="button" onclick={() => (open = false)}>Inner</button>
    </div>
  {:else}
    <div role="menu" aria-label="Overlay" {@attach menuNav({ onclose: () => (open = false) })}>
      <button type="button" role="menuitem" onclick={() => (open = false)}>Item</button>
    </div>
  {/if}
{/if}
