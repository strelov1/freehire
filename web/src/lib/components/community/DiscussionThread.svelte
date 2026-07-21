<script lang="ts">
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { openAuthDialog } from '$lib/auth-dialog.svelte';
  import { communityFormError } from '$lib/community';
  import type { CommunityThread, CommunityReply } from '$lib/types';
  import { Button } from '$lib/ui';
  import { timeAgo } from '$lib/utils';
  import ReplyNode from './ReplyNode.svelte';

  let {
    thread,
    initialReplies,
    initialCursor,
    backPath,
  }: {
    thread: CommunityThread;
    initialReplies: CommunityReply[];
    initialCursor?: string;
    backPath: string;
  } = $props();

  let replies = $state<CommunityReply[]>([...initialReplies]);
  let cursor = $state<string | undefined>(initialCursor);
  let loadingMore = $state(false);
  let count = $state(thread.reply_count);

  let body = $state('');
  let submitting = $state(false);
  let formError = $state<string | null>(null);
  const closed = $derived(thread.status === 'closed');
  const canSubmit = $derived(body.trim() !== '' && !submitting && !closed);

  // Top-level replies (parent_id === 0); ReplyNode renders each subtree from `replies`.
  const roots = $derived(replies.filter((r) => r.parent_id === 0));

  function addReply(r: CommunityReply) {
    replies = [...replies, r];
    count += 1;
  }

  async function loadMore() {
    if (!cursor || loadingMore) return;
    loadingMore = true;
    try {
      const res = await api.getThread(thread.id, cursor);
      replies = [...replies, ...res.replies];
      cursor = res.nextCursor;
    } finally {
      loadingMore = false;
    }
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    submitting = true;
    formError = null;
    try {
      const created = await api.createReply(thread.id, body.trim());
      addReply(created);
      body = '';
    } catch (err) {
      formError = communityFormError(err);
    } finally {
      submitting = false;
    }
  }
</script>

<article class="thread">
  <a class="thread__back" href={backPath}>← Back to discussion</a>

  <header class="thread__head">
    <h1 class="thread__title">{thread.title}</h1>
    <p class="thread__meta">{thread.author} · {timeAgo(thread.created_at)}</p>
  </header>
  <p class="thread__body">{thread.body}</p>

  <section class="replies">
    <h2 class="replies__count">{count} {count === 1 ? 'reply' : 'replies'}</h2>
    {#each roots as root (root.id)}
      <ReplyNode reply={root} {replies} threadId={thread.id} {closed} depth={0} {addReply} />
    {/each}
    {#if cursor}
      <div class="load-more">
        <Button variant="ghost" onclick={loadMore} disabled={loadingMore}>
          {loadingMore ? 'Loading…' : 'Load more'}
        </Button>
      </div>
    {/if}
  </section>

  {#if closed}
    <p class="closed-note">This thread is closed.</p>
  {:else if isAuthenticated()}
    <form class="reply-form" onsubmit={submit}>
      <textarea bind:value={body} rows="3" placeholder="Add a reply…" class="reply-form__body"></textarea>
      {#if formError}<p class="reply-form__error">{formError}</p>{/if}
      <div class="reply-form__actions">
        <Button type="submit" disabled={!canSubmit}>{submitting ? 'Posting…' : 'Reply'}</Button>
      </div>
    </form>
  {:else}
    <p class="signin-hint">
      <button class="linklike" onclick={() => openAuthDialog()}>Sign in</button> to reply.
    </p>
  {/if}
</article>

<style>
  .thread {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  .thread__back {
    color: var(--muted-foreground, #6b7280);
    text-decoration: none;
    font-size: 0.85rem;
  }
  .thread__title {
    margin: 0;
    font-size: 1.4rem;
  }
  .thread__meta {
    color: var(--muted-foreground, #6b7280);
    font-size: 0.8rem;
    margin: 0.15rem 0 0;
  }
  .thread__body {
    margin: 0;
    white-space: pre-wrap;
  }
  .replies {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    border-top: 1px solid var(--border, #e5e7eb);
    padding-top: 1rem;
  }
  .replies__count {
    margin: 0 0 0.25rem;
    font-size: 1rem;
  }
  .reply-form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  .reply-form__body {
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--border, #d1d5db);
    border-radius: 0.5rem;
    font: inherit;
    resize: vertical;
  }
  .reply-form__actions {
    display: flex;
    justify-content: flex-end;
  }
  .reply-form__error {
    color: var(--destructive, #dc2626);
    font-size: 0.85rem;
    margin: 0;
  }
  .closed-note,
  .signin-hint {
    color: var(--muted-foreground, #6b7280);
    font-size: 0.9rem;
  }
  .linklike {
    background: none;
    border: none;
    padding: 0;
    color: var(--primary, #2563eb);
    cursor: pointer;
    font: inherit;
    text-decoration: underline;
  }
  .load-more {
    display: flex;
    justify-content: center;
  }
</style>
