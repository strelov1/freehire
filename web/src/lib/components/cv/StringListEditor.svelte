<script lang="ts">
  import { X, Plus } from '@lucide/svelte';
  import { Button, Input } from '$lib/ui';

  // A small editor for a list of free-text strings (résumé bullets, header links, skill
  // items). Binds the array directly so the parent's Document updates in place.
  let {
    items = $bindable<string[]>([]),
    placeholder = '',
    addLabel = 'Add',
  }: { items: string[]; placeholder?: string; addLabel?: string } = $props();

  function add() {
    items = [...items, ''];
  }
  function remove(i: number) {
    items = items.filter((_, idx) => idx !== i);
  }
</script>

<div class="space-y-2">
  {#each items as _, i (i)}
    <div class="flex items-center gap-2">
      <Input bind:value={items[i]} {placeholder} class="flex-1" />
      <Button variant="ghost" size="icon" onclick={() => remove(i)} aria-label="Remove">
        <X class="h-4 w-4" />
      </Button>
    </div>
  {/each}
  <Button variant="outline" size="sm" onclick={add}>
    <Plus class="mr-1 h-4 w-4" />{addLabel}
  </Button>
</div>
