<script lang="ts">
  import { X, Plus } from '@lucide/svelte';
  import { Button, Input } from '#lib/ui/index.js';

  // A small editor for a list of free-text strings (résumé bullets, header links, skill
  // items). Binds the array directly so the parent's Document updates in place.
  let {
    items = $bindable<string[]>([]),
    placeholder = '',
    addLabel = 'Add',
    // Names each row's input for assistive tech ("Bullet 2"), since the placeholder is
    // the only visible hint and a placeholder is not an accessible name.
    label = 'Item',
  }: { items: string[]; placeholder?: string; addLabel?: string; label?: string } = $props();

  // Stable per-row keys. String values aren't unique (duplicate or empty rows) and
  // rows are removed from the middle, so an index key would rebind surviving inputs
  // by position — jumping focus/cursor/IME onto the wrong row. A parallel id array,
  // mutated in lockstep with `items`, gives each row a stable identity. All
  // structural edits go through add/remove here, so ids.length === items.length holds.
  let ids = $state<number[]>(items.map((_, i) => i));
  let nextId = items.length;

  function add() {
    items = [...items, ''];
    ids = [...ids, nextId++];
  }
  function remove(i: number) {
    items = items.filter((_, idx) => idx !== i);
    ids = ids.filter((_, idx) => idx !== i);
  }
</script>

<div class="space-y-2">
  {#each items as _, i (ids[i])}
    <div class="flex items-center gap-2">
      <Input bind:value={items[i]} {placeholder} aria-label="{label} {i + 1}" class="flex-1" />
      <Button variant="ghost" size="icon" onclick={() => remove(i)} aria-label="Remove {label.toLowerCase()} {i + 1}">
        <X class="h-4 w-4" />
      </Button>
    </div>
  {/each}
  <Button variant="outline" size="sm" onclick={add}>
    <Plus class="mr-1 h-4 w-4" />{addLabel}
  </Button>
</div>
