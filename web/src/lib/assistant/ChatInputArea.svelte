<script lang="ts">
  import { Phone } from '@lucide/svelte';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { Attachment } from './client';
  import Composer from './Composer.svelte';
  import VoiceCall from './VoiceCall.svelte';
  import { messages } from './AssistantChat.messages';

  // AssistantChat's input area: the composer, or a hands-free voice call in its place while
  // `voiceCallOpen`. `canCall` offers the call (an interview session where voice works);
  // `onUnavailable` is told when the server has no Realtime gateway.
  let {
    draft = $bindable(),
    attachments = $bindable(),
    voiceCallOpen = $bindable(),
    sessionId,
    queue,
    turnActive,
    support,
    disabled,
    canCall,
    onSubmit,
    onRemoveQueued,
    onCancel,
    onClose,
    onUnavailable,
  }: {
    draft: string;
    attachments: Attachment[];
    voiceCallOpen: boolean;
    sessionId: string | null;
    queue: { id: string; text: string; attachments: Attachment[] }[];
    turnActive: boolean;
    support: boolean;
    disabled: boolean;
    canCall: boolean;
    onSubmit: (text: string, docs: Attachment[]) => void;
    onRemoveQueued: (id: string) => void;
    onCancel: () => void;
    onClose: () => void;
    onUnavailable: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));
</script>

{#if voiceCallOpen && sessionId}
  <!-- Keyed on activeId: switching sessions while a call is open must destroy
     this instance (its onDestroy ends the call cleanly) rather than reuse it
     with a new sessionId prop, which would silently post the OLD call's turns
     into the NEW session's transcript. -->
  {#key sessionId}
    <div class="border-t border-border p-3">
      <div class="mx-auto w-full max-w-3xl">
        <VoiceCall {sessionId} {onClose} {onUnavailable} />
      </div>
    </div>
  {/key}
{:else}
  <Composer
    bind:draft
    bind:attachments
    {queue}
    {turnActive}
    attachable={!support}
    dictatable={!support}
    {disabled}
    {onSubmit}
    {onRemoveQueued}
    {onCancel}
  />
  {#if canCall && sessionId}
    <div class="flex justify-center border-t border-border/60 py-2">
      <button
        type="button"
        onclick={() => (voiceCallOpen = true)}
        disabled={disabled || turnActive}
        class="flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
      >
        <Phone class="size-3.5" />
        {s.voiceModeButton}
      </button>
    </div>
  {/if}
{/if}
