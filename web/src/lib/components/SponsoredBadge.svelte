<script lang="ts">
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { cn } from '#lib/ui/index.js';
  import { messages } from './Sponsored.messages.js';

  // The disclosure on a paid slot. Visible text, never colour alone, and neutral in tone:
  // it is a label, not a warning.
  let { class: className = '' }: { class?: string } = $props();
  const s = $derived(t(messages, locale()));
</script>

<span
  title={s.labelHint}
  class={cn(
    'inline-flex items-center rounded-md border border-border px-2 py-0.5 text-xs font-medium text-muted-foreground',
    className,
  )}
>
  {s.label}
</span>
