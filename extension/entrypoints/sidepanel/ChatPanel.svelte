<script lang="ts">
  import { ArrowUp, Square } from '@lucide/svelte';
  import { Button, Input } from 'freehire-design-system';
  import type { ChatState } from '../../lib/assistant/chat';
  import { splitPresentingCalls } from '../../lib/assistant/deck';
  import type { HireUser } from '../../lib/auth';
  import type { PlanRefused } from '../../lib/planRefused';
  import JobDeck from './JobDeck.svelte';
  import PlanRefusalAlert from './PlanRefusalAlert.svelte';
  import ToolGroupList from './ToolGroupList.svelte';

  // The side panel's Chat tab: the conversation with the agent and the composer under it.
  // `.message`, `.message.system` and `.empty` are also styled in App.svelte, whose Match
  // tab uses them.
  let {
    user,
    chat,
    chatError,
    planRefusal,
    restoring,
    sending,
    draft = $bindable(),
    onReset,
    onSend,
    onStop,
  }: {
    user: HireUser | null;
    chat: ChatState;
    chatError: string;
    planRefusal: PlanRefused | null;
    restoring: boolean;
    sending: boolean;
    draft: string;
    onReset: () => void;
    onSend: () => void;
    onStop: () => void;
  } = $props();
</script>

<div class="chat-panel">
  {#if user}
    <div class="chat-toolbar">
      <Button variant="ghost" size="sm" onclick={onReset} disabled={sending}>Reset</Button>
    </div>
  {/if}
  <div class="messages">
    {#each chat.messages as message, mi (mi)}
      {@const split = splitPresentingCalls(message.tools, message.streaming)}
      {#each split.decks as slot, di (di)}
        <JobDeck {slot} />
      {/each}
      {#if split.rest.length > 0}
        <ToolGroupList calls={split.rest} />
      {/if}
      {#if message.text || message.streaming}
        <div class="message {message.role}" class:errored={message.errored}>
          {message.text}{#if message.streaming && !message.text}<span class="dots">…</span>{/if}
        </div>
      {/if}
    {/each}
    {#if chatError}
      <div class="message system err">{chatError}</div>
    {/if}
    {#if planRefusal}
      <PlanRefusalAlert refusal={planRefusal} />
    {/if}
    {#if chat.messages.length === 0}
      <p class="empty">
        {#if restoring}
          Loading your conversation…
        {:else if user}
          Ask about the page you're on — the agent can read it.
        {:else}
          Sign in to chat with the agent.
        {/if}
      </p>
    {/if}
  </div>

  <div class="composer">
    <Input
      class="flex-1"
      placeholder={user ? 'Message the agent…' : 'Sign in to chat'}
      bind:value={draft}
      disabled={!user || sending}
      onkeydown={(e) => e.key === 'Enter' && onSend()}
    />
    {#if sending}
      <Button class="rounded-full" variant="primary" size="icon" aria-label="Stop the assistant" onclick={onStop}>
        <Square class="size-3.5" fill="currentColor" />
      </Button>
    {:else}
      <Button
        class="rounded-full"
        variant="primary"
        size="icon"
        aria-label="Send message"
        onclick={onSend}
        disabled={!user}
      >
        <ArrowUp class="size-4" strokeWidth={2.5} />
      </Button>
    {/if}
  </div>
</div>

<style>
  .chat-panel {
    flex: 1;
    min-height: 0;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  .messages {
    flex: 1;
    min-height: 0;
    min-width: 0;
    overflow-y: auto;
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  /* Same fix as .match-panel above, same reason: without it a long transcript
   * would squash individual messages instead of just scrolling past them. */
  .messages > :global(*) {
    flex-shrink: 0;
  }

  .empty {
    color: var(--muted-foreground);
    text-align: center;
    margin-top: 40px;
  }

  .message {
    padding: 8px 10px;
    border-radius: 8px;
    max-width: 85%;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .message.user {
    align-self: flex-end;
    background: var(--brand);
    color: var(--brand-foreground);
  }

  .message.assistant {
    align-self: flex-start;
    background: var(--muted);
    color: var(--foreground);
  }

  .message.system {
    align-self: center;
    background: var(--warning-muted);
    font-size: 12px;
    color: var(--warning-strong);
  }

  .message.system.err {
    background: color-mix(in srgb, var(--destructive) 8%, transparent);
    color: var(--destructive);
  }

  .message.assistant.errored {
    border: 1px solid color-mix(in srgb, var(--destructive) 40%, transparent);
  }

  .dots {
    opacity: 0.5;
  }

  .chat-toolbar {
    display: flex;
    justify-content: flex-end;
    padding: 6px 10px 0;
    flex-shrink: 0;
  }

  .composer {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px;
    border-top: 1px solid var(--border);
  }
</style>
