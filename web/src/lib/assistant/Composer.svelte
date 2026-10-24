<script lang="ts">
  import { ArrowUp, FileText, Loader2, Paperclip, Trash2, Square, X } from '@lucide/svelte';
  import { Chip } from '#lib/ui/index.js';
  import { extractAttachment, type Attachment } from '#lib/assistant/client.js';
  import VoiceInput from '#lib/assistant/VoiceInput.svelte';
  import { DICTATION_ENABLED } from '#lib/assistant/audioAvailability.js';
  import { appendTranscript } from '#lib/assistant/dictation.js';
  import { messages } from './Composer.messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { format, t } from '#lib/i18n/t.js';

  // The composer: the queued-message panel (messages typed mid-turn, sent
  // one-by-one as turns finish) plus the auto-growing textarea form. Queue and
  // dispatch orchestration stays with the parent — this owns the draft, the
  // textarea autosize, and the rendering; `disabled` folds the parent's
  // not-ready / connection-lost / switching states into one flag.
  let {
    draft = $bindable(''),
    attachments = $bindable([]),
    attachable = true,
    queue,
    turnActive,
    disabled,
    onSubmit,
    onRemoveQueued,
    onCancel = undefined,
  }: {
    draft?: string;
    /** Documents read for the next message, sent with it and then cleared by the parent. */
    attachments?: Attachment[];
    /** Whether to offer the attach control at all. Off for a host that reads only the
     *  text, so a chosen file is never silently dropped. */
    attachable?: boolean;
    queue: { id: string; text: string; attachments: Attachment[] }[];
    turnActive: boolean;
    disabled: boolean;
    onSubmit: (text: string, attachments: Attachment[]) => void;
    onRemoveQueued: (id: string) => void;
    /** Stop the turn in flight. Absent when the host cannot cancel. */
    onCancel?: () => void;
  } = $props();

  const s = $derived(t(messages, locale()));
  let textareaEl = $state<HTMLTextAreaElement | null>(null);

  // Dictation. `dictationOff` latches once the server reports no speech gateway: the
  // feature is absent in this deployment, and offering a microphone that answers 501
  // every time is worse than offering none. `voiceError` is shown in place of the
  // placeholder hint, so a denied permission is legible without a toast system.
  let dictationOff = $state(false);
  let voiceError = $state<string | null>(null);

  function acceptTranscript(text: string) {
    voiceError = null;
    draft = appendTranscript(draft, text);
    // Focus follows the words: the caller's next act is to edit or to send, and both
    // need the cursor here. They send it themselves — nothing is dispatched for them.
    textareaEl?.focus();
  }

  /** Put the cursor in the message field, for a host whose own control means "type here
   *  next" — so it need not reach into this component's markup to find the field. */
  export function focus() {
    textareaEl?.focus();
  }

  // Auto-grow the composer textarea up to a cap (px), like roy-web's `autosize`.
  const COMPOSER_CAP = 200;
  $effect(() => {
    void draft;
    const el = textareaEl;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, COMPOSER_CAP)}px`;
  });

  // Attachments. A PDF is read by the server the moment it is chosen, not when the message
  // is sent: a scan with no text layer is then refused while the candidate is still looking
  // at the file picker, rather than after they have written the message that refers to it.
  // The server keeps nothing — the extracted text rides along with the message.
  const MAX_ATTACHMENTS = 3;
  let fileInput = $state<HTMLInputElement | null>(null);
  let reading = $state(false);
  let attachError = $state<string | null>(null);
  const canAttach = $derived(!disabled && !reading && attachments.length < MAX_ATTACHMENTS);
  const hasContent = $derived(draft.trim() !== '' || attachments.length > 0);

  async function attachFile(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    // Cleared at once so choosing the same file again after removing it still fires `change`.
    input.value = '';
    if (!file) return;
    attachError = null;
    reading = true;
    try {
      const doc = await extractAttachment(file);
      if (attachments.length < MAX_ATTACHMENTS) attachments = [...attachments, doc];
    } catch (err) {
      attachError = err instanceof Error ? err.message : s.attachFailed;
    } finally {
      reading = false;
    }
  }

  function removeAttachment(index: number) {
    attachments = attachments.filter((_, i) => i !== index);
  }

  function submit(e: SubmitEvent) {
    e.preventDefault();
    const text = draft.trim();
    if (!hasContent || disabled || reading) return;
    attachError = null;
    onSubmit(text, attachments);
  }
</script>

<div class="border-t border-border p-3">
  <div class="mx-auto w-full max-w-3xl">
    {#if queue.length > 0}
      <!-- Queued messages: sent one-by-one as each turn finishes. -->
      <div class="mb-2 overflow-hidden rounded-2xl border border-border/60 bg-card">
        <div class="px-4 py-2 text-xs font-medium text-brand">{format(s.queuedCount, { count: String(queue.length) })}</div>
        <ul class="divide-y divide-border/40 border-t border-border/40">
          {#each queue as item (item.id)}
            <li class="group flex items-start gap-3 px-4 py-2">
              <span class="min-w-0 flex-1 whitespace-pre-wrap break-words text-sm text-foreground">
                {item.text}
                {#each item.attachments as doc, i (i)}
                  <Chip class="ml-1 gap-1 align-middle"><FileText class="size-3" />{doc.name}</Chip>
                {/each}
              </span>
              <button
                type="button"
                aria-label={s.removeFromQueueAria}
                title={s.removeTitle}
                onclick={() => onRemoveQueued(item.id)}
                class="flex size-7 shrink-0 items-center justify-center rounded-md text-muted-foreground opacity-60 transition-opacity hover:bg-muted hover:text-foreground group-hover:opacity-100"
              >
                <Trash2 class="size-4" />
              </button>
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    {#if attachments.length > 0 || reading}
      <ul class="mb-2 flex flex-wrap gap-2" aria-label={s.attachmentsAria}>
        {#each attachments as doc, i (i)}
          <li>
            <Chip class="max-w-full gap-1.5 py-1 pr-1">
              <FileText class="size-3.5 shrink-0" />
              <span class="truncate" title={doc.name}>{doc.name}</span>
              {#if doc.truncated}
                <span class="shrink-0 text-muted-foreground" title={s.truncatedTitle}>{s.truncated}</span>
              {/if}
              <button
                type="button"
                aria-label={format(s.removeAttachmentAria, { name: doc.name })}
                onclick={() => removeAttachment(i)}
                class="flex size-5 shrink-0 items-center justify-center rounded-full text-muted-foreground hover:bg-background hover:text-foreground"
              >
                <X class="size-3" />
              </button>
            </Chip>
          </li>
        {/each}
        {#if reading}
          <li>
            <Chip class="gap-1.5 py-1"><Loader2 class="size-3.5 animate-spin" />{s.reading}</Chip>
          </li>
        {/if}
      </ul>
    {/if}

    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <form
      onsubmit={submit}
      onclick={(e) => {
        if ((e.target as HTMLElement).closest('button')) return;
        textareaEl?.focus();
      }}
      class="relative flex w-full cursor-text items-end gap-2 rounded-3xl border border-border/60 bg-card px-4 py-2.5 shadow-sm transition-colors focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/50"
    >
      {#if attachable}
      <input
        bind:this={fileInput}
        type="file"
        accept="application/pdf,.pdf"
        class="hidden"
        tabindex="-1"
        aria-hidden="true"
        onchange={attachFile}
      />
      <button
        type="button"
        aria-label={s.attachAria}
        title={s.attachTitle}
        disabled={!canAttach}
        onclick={() => fileInput?.click()}
        class="flex size-9 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
      >
        <Paperclip class="size-4" />
      </button>
      {/if}
      <textarea
        bind:this={textareaEl}
        bind:value={draft}
        rows="1"
        placeholder={s.placeholder}
        aria-label={s.messageAria}
        {disabled}
        onkeydown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            (e.currentTarget.form as HTMLFormElement | null)?.requestSubmit();
          }
        }}
        class="block max-h-[200px] min-h-[1.5rem] flex-1 resize-none cursor-text bg-transparent py-1 text-base leading-6 text-foreground placeholder:text-muted-foreground/70 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50 sm:text-sm"
      ></textarea>
      {#if DICTATION_ENABLED && !dictationOff}
        <VoiceInput
          {disabled}
          onTranscript={acceptTranscript}
          onError={(message) => (voiceError = message)}
          onUnavailable={() => (dictationOff = true)}
        />
      {/if}
      {#if turnActive && onCancel && !hasContent}
        <button
          type="button"
          aria-label={s.stopAria}
          onclick={onCancel}
          class="flex size-9 shrink-0 items-center justify-center rounded-full bg-foreground text-background transition-opacity hover:opacity-90"
        >
          <Square class="size-3.5" fill="currentColor" />
        </button>
      {:else}
        <button
          type="submit"
          aria-label={turnActive ? s.queueMessageAria : s.sendMessageAria}
          aria-busy={turnActive}
          disabled={disabled || reading || !hasContent}
          class="flex size-9 shrink-0 items-center justify-center rounded-full bg-foreground text-background transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {#if turnActive}
            <Loader2 class="size-4 animate-spin" />
          {:else}
            <ArrowUp class="size-4" strokeWidth={2.5} />
          {/if}
        </button>
      {/if}
    </form>

    {#if voiceError}
      <p class="mt-1.5 px-4 text-xs text-destructive" role="alert">{voiceError}</p>
    {/if}
    {#if attachError}
      <p class="mt-1.5 px-4 text-xs text-destructive" role="alert">{attachError}</p>
    {/if}
  </div>
</div>
