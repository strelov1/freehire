<script lang="ts">
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './SkipLink.messages';

  // The page's first focusable element: lets a keyboard user jump past the header's
  // navigation. Hidden until focused, so only the people who Tab ever see it.
  let { target }: { target: string } = $props();

  const s = $derived(t(messages, locale()));
</script>

<a
  href="#{target}"
  class="sr-only focus:not-sr-only focus:fixed focus:left-3 focus:top-3 focus:z-toast focus:rounded-md focus:border focus:border-border focus:bg-background focus:px-3 focus:py-2 focus:text-sm focus:font-medium focus:text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
>
  {s.skipToContent}
</a>
