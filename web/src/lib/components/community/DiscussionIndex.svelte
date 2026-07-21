<script lang="ts">
  import { api } from '$lib/api';
  import type { CommunityThread } from '$lib/types';
  import { Button } from '$lib/ui';
  import { timeAgo } from '$lib/utils';

  let {
    subjectType,
    subjectSlug,
    basePath,
    initialThreads,
    initialCursor,
  }: {
    subjectType: string;
    subjectSlug: string;
    /** Route prefix a thread links off, e.g. "/companies/acme/discussion". */
    basePath: string;
    initialThreads: CommunityThread[];
    initialCursor?: string;
  } = $props();

  let threads = $state<CommunityThread[]>([...initialThreads]);
  let cursor = $state<string | undefined>(initialCursor);
  let loadingMore = $state(false);

  async function loadMore() {
    if (!cursor || loadingMore) return;
    loadingMore = true;
    try {
      const res = await api.listThreads(subjectType, subjectSlug, cursor);
      threads = [...threads, ...res.threads];
      cursor = res.nextCursor;
    } finally {
      loadingMore = false;
    }
  }
</script>

<section class="discussion">
  <header class="discussion__head">
    <div>
      <h2>Discussion</h2>
      <p class="discussion__sub">Anonymous — you post under a pseudonym, not your name.</p>
    </div>
    <Button href={`${basePath}/new`}>Start a topic</Button>
  </header>

  {#if threads.length === 0}
    <p class="empty">No topics yet — be the first to start one.</p>
  {:else}
    <ul class="thread-list">
      {#each threads as t (t.id)}
        <li class="thread-list__item">
          <a class="thread-list__link" href={`${basePath}/${t.id}`}>
            <span class="thread-list__title">{t.title}</span>
            <span class="thread-list__meta">
              {t.author} · {t.reply_count} {t.reply_count === 1 ? 'reply' : 'replies'} · {timeAgo(t.created_at)}
            </span>
          </a>
        </li>
      {/each}
    </ul>
    {#if cursor}
      <div class="load-more">
        <Button variant="ghost" onclick={loadMore} disabled={loadingMore}>
          {loadingMore ? 'Loading…' : 'Load more'}
        </Button>
      </div>
    {/if}
  {/if}
</section>

<style>
  .discussion {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }
  .discussion__head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 1rem;
  }
  .discussion__head h2 {
    margin: 0;
    font-size: 1.25rem;
  }
  .discussion__sub {
    margin: 0.25rem 0 0;
    color: var(--muted-foreground, #6b7280);
    font-size: 0.85rem;
  }
  .empty {
    color: var(--muted-foreground, #6b7280);
  }
  .thread-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .thread-list__link {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    padding: 0.6rem 0.75rem;
    border: 1px solid var(--border, #e5e7eb);
    border-radius: 0.5rem;
    text-decoration: none;
    color: inherit;
  }
  .thread-list__link:hover {
    background: var(--muted, #f9fafb);
  }
  .thread-list__title {
    font-weight: 600;
  }
  .thread-list__meta {
    color: var(--muted-foreground, #6b7280);
    font-size: 0.8rem;
  }
  .load-more {
    display: flex;
    justify-content: center;
  }
</style>
