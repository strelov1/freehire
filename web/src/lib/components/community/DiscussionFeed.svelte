<script lang="ts">
  import { resolve } from '$app/paths';
  import { MessageSquare } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { feedSubjectLine } from '$lib/feedSubject';
  import { companyLogoUrl } from '$lib/logo';
  import type { CommunityFeedThread } from '$lib/types';
  import { Button, EntityLogo } from '$lib/ui';
  import { timeAgo } from '$lib/utils';

  let {
    initialThreads,
    initialCursor,
  }: {
    initialThreads: CommunityFeedThread[];
    initialCursor?: string;
  } = $props();

  let threads = $state<CommunityFeedThread[]>([...initialThreads]);
  let cursor = $state<string | undefined>(initialCursor);
  let loadingMore = $state(false);

  async function loadMore() {
    if (!cursor || loadingMore) return;
    loadingMore = true;
    try {
      const res = await api.listRecentThreads(cursor);
      threads = [...threads, ...res.threads];
      cursor = res.nextCursor;
    } finally {
      loadingMore = false;
    }
  }

</script>

{#if threads.length === 0}
  <p class="text-muted-foreground">No discussions yet.</p>
{:else}
  <ul class="flex flex-col gap-2">
    {#each threads as t (t.id)}
      {@const subject = feedSubjectLine(t)}
      <li>
        <!-- Reddit's post anatomy, which is also this codebase's card anatomy (see
             JobRow): a quiet subject rail on top, the title as the hero, a body
             preview, then the reply/author footer. The one departure is the logo,
             which sits in a left rail spanning the card rather than inline in the
             rail, so a column of rows scans by employer down the left edge. -->
        <!-- The row links to the thread's page under its OWN subject, so the feed adds
             no second reading surface. Resolved inline rather than in a helper (as
             DiscussionIndex does) because both the typed-route check and the
             no-navigation-without-resolve lint rule read the href expression itself. -->
        <a
          href={t.subject_type === 'company'
            ? resolve('/companies/[slug]/discussion/[threadId]', {
                slug: t.subject_slug,
                threadId: String(t.id),
              })
            : resolve('/jobs/[slug]/discussion/[threadId]', {
                slug: t.subject_slug,
                threadId: String(t.id),
              })}
          class="flex gap-3 rounded-lg border border-border bg-card p-4 transition-colors hover:bg-muted/50"
        >
          <EntityLogo
            name={t.subject_company}
            src={companyLogoUrl(t.subject_company) ?? undefined}
            shape="square"
            size="md"
            class="mt-0.5 shrink-0"
          />

          <div class="min-w-0 flex-1">
            <!-- The subject rail. The employer holds its width; the posting title is
                 what truncates, since it is the longer and the more expendable of
                 the two. An unresolved subject prints its stored slug — the row
                 stays readable and linkable rather than being dropped. -->
            <div class="flex items-baseline gap-2 text-sm">
              <span class="min-w-0 truncate text-muted-foreground">
                <span class="font-medium text-foreground">{subject.employer}</span>
                {#if subject.posting}
                  <!-- Non-breaking spaces, not plain ones: Svelte trims whitespace at
                       an element boundary, which left the separator glued to the
                       posting title ("Stripe ·Staff Software Engineer"). They also
                       keep the dot from starting a wrapped line on its own. -->
                  <span aria-hidden="true">&nbsp;·&nbsp;</span>{subject.posting}
                {/if}
              </span>
              <span class="ml-auto shrink-0 text-xs tabular-nums text-muted-foreground">
                {timeAgo(t.created_at)}
              </span>
            </div>

            <h2 class="mt-1.5 line-clamp-2 text-lg font-semibold leading-snug tracking-tight">
              {t.title}
            </h2>

            {#if t.body}
              <p class="mt-1 line-clamp-2 text-sm text-muted-foreground">{t.body}</p>
            {/if}

            <div class="mt-2.5 flex items-center gap-1.5 text-xs text-muted-foreground">
              <MessageSquare class="size-3.5" aria-hidden="true" />
              <span>{t.reply_count} {t.reply_count === 1 ? 'reply' : 'replies'}</span>
              <span aria-hidden="true">·</span>
              <span class="truncate">{t.author}</span>
            </div>
          </div>
        </a>
      </li>
    {/each}
  </ul>

  {#if cursor}
    <div class="mt-4 flex justify-center">
      <Button variant="ghost" onclick={loadMore} disabled={loadingMore}>
        {loadingMore ? 'Loading…' : 'Load more'}
      </Button>
    </div>
  {/if}
{/if}
