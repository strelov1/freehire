<script lang="ts">
  import { BookmarkMinus } from '@lucide/svelte';
  import type { MyJob } from '#lib/types.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';
  import { messages } from './BoardRemoveButton.messages';

  let { item, onremove, disabled = false }: {
    item: MyJob;
    onremove: (item: MyJob) => void;
    disabled?: boolean;
  } = $props();
  const title = $derived(item.job?.title || item.role_title);
  const s = $derived(t(messages, locale()));
</script>

<button
  type="button"
  {disabled}
  onclick={() => onremove(item)}
  aria-label={format(s.removeAria, { title })}
  title={s.removeTitle}
  class="inline-flex size-7 shrink-0 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"
>
  <BookmarkMinus class="size-4" aria-hidden="true" />
</button>
