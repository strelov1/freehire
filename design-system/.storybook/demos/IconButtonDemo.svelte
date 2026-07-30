<script lang="ts">
  // `size: 'icon'` is the one Button size whose content is not text, and an icon
  // button carries no accessible name unless the call site gives it one — worth
  // showing together rather than as a square with a glyph in it.
  import Button from '../../src/button.svelte';
  import { X } from '@lucide/svelte';

  let { variant = 'ghost' }: { variant?: 'primary' | 'secondary' | 'outline' | 'ghost' } = $props();
</script>

<Button {variant} size="icon" aria-label="Dismiss">
  <X class="size-4" />
</Button>
