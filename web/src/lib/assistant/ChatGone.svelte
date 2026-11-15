<script lang="ts">
  import { resolve } from '$app/paths';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { messages } from './AssistantChat.messages';

  // What AssistantChat shows for an address naming a conversation that is not the caller's
  // to open (deleted, someone else's, or bound to a CV): a dead link, so an explanation
  // and the way back to their chats rather than an error.
  const s = $derived(t(messages, locale()));
</script>

<div class="m-3 rounded-xl border border-border bg-card p-8 text-center">
  <p class="text-sm text-muted-foreground">
    {s.chatGone}
  </p>
  <a
    href={resolve('/my/assistant/[[id]]', {})}
    class="mt-4 inline-flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm font-medium transition-colors hover:bg-muted"
  >
    {s.openYourChats}
  </a>
</div>
