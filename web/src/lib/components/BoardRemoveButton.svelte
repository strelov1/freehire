<script lang="ts">
  import { BookmarkMinus } from '@lucide/svelte';
  import type { MyJob } from '#lib/types.js';

  let { item, onremove, disabled = false }: {
    item: MyJob;
    onremove: (item: MyJob) => void;
    disabled?: boolean;
  } = $props();
  const title = $derived(item.job?.title || item.role_title);
</script>

<button
  type="button"
  {disabled}
  onclick={() => onremove(item)}
  aria-label="Remove {title} from board"
  title="Remove from board"
  class="inline-flex size-7 shrink-0 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"
>
  <BookmarkMinus class="size-4" aria-hidden="true" />
</button>
