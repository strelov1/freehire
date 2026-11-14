<script lang="ts">
  import { FileText } from '@lucide/svelte';
  import type { ChatMessage } from '#lib/assistant/chat.js';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import { Chip } from '#lib/ui/index.js';
  import { messages } from './AssistantChat.messages';

  // One of the candidate's own messages in AssistantChat's transcript: its text and the
  // names of the files attached to it.
  let { message }: { message: ChatMessage } = $props();

  const s = $derived(t(messages, locale()));
</script>

<article
  class="self-end max-w-[80%] rounded-2xl rounded-br-md bg-secondary px-4 py-2.5 text-sm leading-relaxed text-secondary-foreground"
>
  {#if message.text}
    <pre class="m-0 whitespace-pre-wrap break-words font-sans">{message.text}</pre>
  {/if}
  {#if message.attachments.length > 0}
    <ul class={['flex flex-wrap gap-1.5', message.text && 'mt-2']} aria-label={s.attachedFiles}>
      {#each message.attachments as name, j (j)}
        <li>
          <Chip class="max-w-full gap-1 bg-background"
            ><FileText class="size-3 shrink-0" /><span class="truncate">{name}</span></Chip
          >
        </li>
      {/each}
    </ul>
  {/if}
</article>
