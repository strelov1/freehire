<script lang="ts">
  import { onMount } from 'svelte';
  import { Phone } from '@lucide/svelte';
  import { VOICE_CALL_ENABLED } from './audioAvailability';
  import { canUseVoiceCall } from './voiceCall';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';
  import type { Attachment } from './client';
  import Composer from './Composer.svelte';
  import { messages } from './AssistantChat.messages';

  // AssistantChat's input area: the composer, or a hands-free voice call in its place while
  // `voiceCallOpen`, offered on an `interview` session where this browser can hold one.
  let {
    draft = $bindable(),
    attachments = $bindable(),
    voiceCallOpen = $bindable(),
    sessionId,
    queue,
    turnActive,
    support,
    disabled,
    interview,
    onSubmit,
    onRemoveQueued,
    onCancel,
    onClose,
  }: {
    draft: string;
    attachments: Attachment[];
    voiceCallOpen: boolean;
    sessionId: string | null;
    queue: { id: string; text: string; attachments: Attachment[] }[];
    turnActive: boolean;
    support: boolean;
    disabled: boolean;
    interview: boolean;
    onSubmit: (text: string, docs: Attachment[]) => void;
    onRemoveQueued: (id: string) => void;
    onCancel: () => void;
    onClose: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));

  // Voice mode: a hands-free call on an `interview` session, replacing the composer
  // for its duration. `voiceModeOff` latches once the server reports no Realtime
  // gateway (501) — same reasoning as Composer's `dictationOff` — so the trigger does
  // not keep offering a call that can only fail. `voiceSupported` starts false and is
  // decided after mount, the same reasoning VoiceInput.svelte's `supported` gives:
  // reading navigator during SSR would disagree with the client on the first paint.
  // It also carries VOICE_CALL_ENABLED — see audioAvailability.ts.
  let voiceModeOff = $state(false);
  let voiceSupported = $state(false);
  const canCall = $derived(interview && voiceSupported && !voiceModeOff);
  onMount(() => {
    voiceSupported =
      VOICE_CALL_ENABLED &&
      canUseVoiceCall({
        mediaDevices: navigator.mediaDevices,
        RTCPeerConnection: typeof RTCPeerConnection === 'undefined' ? undefined : RTCPeerConnection,
      });
  });
  function onUnavailable() {
    voiceModeOff = true;
    onClose();
  }
</script>

{#if voiceCallOpen && sessionId}
  <!-- Keyed on activeId: switching sessions while a call is open must destroy
     this instance (its onDestroy ends the call cleanly) rather than reuse it
     with a new sessionId prop, which would silently post the OLD call's turns
     into the NEW session's transcript. -->
  {#key sessionId}
    <div class="border-t border-border p-3">
      <div class="mx-auto w-full max-w-3xl">
        {#await import('./VoiceCall.svelte') then { default: VoiceCall }}
          <VoiceCall {sessionId} {onClose} {onUnavailable} />
        {/await}
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
