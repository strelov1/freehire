<script lang="ts">
  import { ArrowUp, Loader2, Trash2, Square } from '@lucide/svelte';
  import VoiceInput from '$lib/assistant/VoiceInput.svelte';
  import { AUDIO_ENABLED } from '$lib/assistant/audioAvailability';
  import { appendTranscript } from '$lib/assistant/dictation';

  // The composer: the queued-message panel (messages typed mid-turn, sent
  // one-by-one as turns finish) plus the auto-growing textarea form. Queue and
  // dispatch orchestration stays with the parent — this owns the draft, the
  // textarea autosize, and the rendering; `disabled` folds the parent's
  // not-ready / connection-lost / switching states into one flag.
  let {
    draft = $bindable(''),
    queue,
    turnActive,
    disabled,
    onSubmit,
    onRemoveQueued,
    onCancel = undefined,
  }: {
    draft?: string;
    queue: { id: string; text: string }[];
    turnActive: boolean;
    disabled: boolean;
    onSubmit: (text: string) => void;
    onRemoveQueued: (id: string) => void;
    /** Stop the turn in flight. Absent when the host cannot cancel. */
    onCancel?: () => void;
  } = $props();

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

  // Auto-grow the composer textarea up to a cap (px), like roy-web's `autosize`.
  const COMPOSER_CAP = 200;
  $effect(() => {
    void draft;
    const el = textareaEl;
    if (!el) return;
    el.style.height = 'auto';
    el.style.height = `${Math.min(el.scrollHeight, COMPOSER_CAP)}px`;
  });

  function submit(e: SubmitEvent) {
    e.preventDefault();
    const text = draft.trim();
    if (!text || disabled) return;
    onSubmit(text);
  }
</script>

<div class="border-t border-border p-3">
  <div class="mx-auto w-full max-w-3xl">
    {#if queue.length > 0}
      <!-- Queued messages: sent one-by-one as each turn finishes. -->
      <div class="mb-2 overflow-hidden rounded-2xl border border-border/60 bg-card">
        <div class="px-4 py-2 text-xs font-medium text-brand">{queue.length} queued</div>
        <ul class="divide-y divide-border/40 border-t border-border/40">
          {#each queue as item (item.id)}
            <li class="group flex items-start gap-3 px-4 py-2">
              <span class="min-w-0 flex-1 whitespace-pre-wrap break-words text-sm text-foreground">{item.text}</span>
              <button
                type="button"
                aria-label="Remove from queue"
                title="Remove"
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

    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <form
      onsubmit={submit}
      onclick={(e) => {
        if ((e.target as HTMLElement).closest('button')) return;
        textareaEl?.focus();
      }}
      class="relative flex w-full cursor-text items-end gap-2 rounded-3xl border border-border/60 bg-card px-4 py-2.5 shadow-sm transition-colors focus-within:border-ring/60"
    >
      <textarea
        bind:this={textareaEl}
        bind:value={draft}
        rows="1"
        placeholder="Message the agent — Enter to send, Shift+Enter for newline"
        {disabled}
        onkeydown={(e) => {
          if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            (e.currentTarget.form as HTMLFormElement | null)?.requestSubmit();
          }
        }}
        class="block max-h-[200px] min-h-[1.5rem] flex-1 resize-none cursor-text bg-transparent py-1 text-base leading-6 text-foreground placeholder:text-muted-foreground/70 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50 sm:text-sm"
      ></textarea>
      {#if AUDIO_ENABLED && !dictationOff}
        <VoiceInput
          {disabled}
          onTranscript={acceptTranscript}
          onError={(message) => (voiceError = message)}
          onUnavailable={() => (dictationOff = true)}
        />
      {/if}
      {#if turnActive && onCancel && !draft.trim()}
        <button
          type="button"
          aria-label="Stop the assistant"
          onclick={onCancel}
          class="flex size-9 shrink-0 items-center justify-center rounded-full bg-foreground text-background transition-opacity hover:opacity-90"
        >
          <Square class="size-3.5" fill="currentColor" />
        </button>
      {:else}
        <button
          type="submit"
          aria-label={turnActive ? 'Queue message' : 'Send message'}
          aria-busy={turnActive}
          disabled={disabled || !draft.trim()}
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
  </div>
</div>
