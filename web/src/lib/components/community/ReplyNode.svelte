<script lang="ts">
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { communityFormError } from '$lib/community';
  import type { CommunityReply } from '$lib/types';
  import { Button } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import Self from './ReplyNode.svelte';

  let {
    reply,
    replies,
    threadId,
    closed,
    depth,
    addReply,
  }: {
    reply: CommunityReply;
    /** The whole flat reply list; children are derived from it by parent_id. */
    replies: CommunityReply[];
    threadId: number;
    closed: boolean;
    depth: number;
    addReply: (r: CommunityReply) => void;
  } = $props();

  // Direct children of this reply, oldest first (the flat list is already ordered).
  const children = $derived(replies.filter((r) => r.parent_id === reply.id));
  // Indentation is capped so deep chains don't march off the right edge.
  const indent = $derived(Math.min(depth, 6) * 16);

  let replying = $state(false);
  let body = $state('');
  let submitting = $state(false);
  let formError = $state<string | null>(null);
  const canSubmit = $derived(body.trim() !== '' && !submitting);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    submitting = true;
    formError = null;
    try {
      const created = await api.createReply(threadId, body.trim(), reply.id);
      addReply(created);
      body = '';
      replying = false;
    } catch (err) {
      formError = communityFormError(err);
    } finally {
      submitting = false;
    }
  }

  function onReplyClick() {
    if (!isAuthenticated()) {
      openAuthDialog();
      return;
    }
    replying = !replying;
  }
</script>

<div class="node" style={`margin-left:${indent}px`}>
  <p class="node__meta">{reply.author} · {timeAgo(reply.created_at)}</p>
  <p class="node__body">{reply.body}</p>

  {#if !closed}
    <button class="node__reply-btn" onclick={onReplyClick}>{replying ? 'Cancel' : 'Reply'}</button>
  {/if}

  {#if replying}
    <form class="node__form" onsubmit={submit}>
      <textarea bind:value={body} rows="2" placeholder="Reply…" class="node__form-body"></textarea>
      {#if formError}<p class="node__error">{formError}</p>{/if}
      <div class="node__form-actions">
        <Button type="submit" disabled={!canSubmit}>{submitting ? 'Posting…' : 'Reply'}</Button>
      </div>
    </form>
  {/if}
</div>

{#each children as child (child.id)}
  <Self reply={child} {replies} {threadId} {closed} depth={depth + 1} {addReply} />
{/each}

<style>
  .node {
    border-left: 2px solid var(--border, #e5e7eb);
    padding: 0.25rem 0 0.25rem 0.75rem;
    margin-top: 0.5rem;
  }
  .node__meta {
    color: var(--muted-foreground, #6b7280);
    font-size: 0.8rem;
    margin: 0;
  }
  .node__body {
    margin: 0.15rem 0 0;
    white-space: pre-wrap;
  }
  .node__reply-btn {
    background: none;
    border: none;
    padding: 0.15rem 0;
    margin-top: 0.15rem;
    color: var(--muted-foreground, #6b7280);
    cursor: pointer;
    font: inherit;
    font-size: 0.8rem;
  }
  .node__reply-btn:hover {
    color: var(--primary, #2563eb);
    text-decoration: underline;
  }
  .node__form {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    margin-top: 0.4rem;
  }
  .node__form-body {
    width: 100%;
    padding: 0.4rem 0.6rem;
    border: 1px solid var(--border, #d1d5db);
    border-radius: 0.5rem;
    font: inherit;
    resize: vertical;
  }
  .node__form-actions {
    display: flex;
    justify-content: flex-end;
  }
  .node__error {
    color: var(--destructive, #dc2626);
    font-size: 0.8rem;
    margin: 0;
  }
</style>
