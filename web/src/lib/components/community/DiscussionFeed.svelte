<script lang="ts">
  import { resolve } from '$app/paths';
  import { MessageSquare } from '@lucide/svelte';
  import { api } from '$lib/api';
  import { feedSubjectLine } from '$lib/feedSubject';
  import { companyLogoUrl } from '$lib/logo';
  import type { CommunityFeedThread } from '$lib/types';
  import { Badge, EntityLogo, LoadMore } from '$lib/ui';
  import { timeAgo } from '$lib/utils';

  let {
    initialThreads,
    initialCursor,
    failed = false,
  }: {
    initialThreads: CommunityFeedThread[];
    initialCursor?: string;
    /** The first page could not be fetched. Distinct from an empty feed, which is a
     *  statement about the catalogue rather than about our reachability. */
    failed?: boolean;
  } = $props();

  let threads = $state<CommunityFeedThread[]>([...initialThreads]);
  let cursor = $state<string | undefined>(initialCursor);
  let loadingMore = $state(false);
  let loadFailed = $state(false);

  async function loadMore() {
    if (!cursor || loadingMore) return;
    loadingMore = true;
    loadFailed = false;
    try {
      const res = await api.listRecentThreads(cursor);
      threads = [...threads, ...res.threads];
      cursor = res.nextCursor;
    } catch {
      // Surfaced through LoadMore's error state and the cursor is left in place, so
      // the button stays and the same page can be retried. Swallowing this silently
      // (which the first cut did) makes a failed fetch look like the end of the list.
      loadFailed = true;
    } finally {
      loadingMore = false;
    }
  }
</script>

{#if failed}
  <p class="text-muted-foreground">
    Discussions couldn't be loaded just now. Please try again in a moment.
  </p>
{:else if threads.length === 0}
  <p class="text-muted-foreground">No discussions yet.</p>
{:else}
  <ul class="flex flex-col gap-1.5">
    {#each threads as t (t.id)}
      {@const subject = feedSubjectLine(t)}
      <li>
        <!-- Reddit's post anatomy, which is also this codebase's card anatomy (see
             JobRow): a quiet subject rail on top, the title as the hero, a body
             preview, then the reply/author footer. The one departure is the logo,
             which sits in a left rail spanning the card rather than inline in the
             rail, so a column of rows scans by employer down the left edge.

             The row links to the thread's page under its OWN subject, so the feed adds
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
          class="flex gap-3 rounded-lg border border-border bg-card px-3.5 py-3 transition-colors hover:bg-muted/50"
        >
          <!-- Keyed on the raw employer name, never on `subject.employer`: that one
               may be the "Unknown company" stand-in or a slug, and the proxy would
               resolve a mark for the literal. Empty here is the nameless tile, which
               is the honest answer for both an absent employer and a gone subject. -->
          <EntityLogo
            name={t.subject_company}
            src={t.subject_company ? (companyLogoUrl(t.subject_company) ?? undefined) : undefined}
            shape="square"
            size="sm"
            class="mt-0.5 shrink-0"
          />

          <div class="min-w-0 flex-1">
            <!-- The subject rail. The kind marks every row, so it reads as the row's
                 type rather than as a warning on one of them. The employer holds its
                 width; the posting title is what truncates, being the longer and the
                 more expendable of the two. -->
            <div class="flex items-center gap-1.5 text-sm">
              <Badge variant="outline" class="shrink-0">{subject.kind}</Badge>
              <span class="min-w-0 truncate text-muted-foreground">
                {#if subject.resolved}
                  <span class="font-medium text-foreground">{subject.employer}</span>
                {:else}
                  <!-- The subject is gone, so this is its stored identifier, not a
                       name. Rendered as one — muted and monospaced — because in the
                       employer's own styling a slug reads as what the employer is
                       called. -->
                  <span class="font-mono text-xs" title="This subject no longer exists">
                    {subject.employer}
                  </span>
                {/if}
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

            <h2 class="mt-1 line-clamp-1 font-semibold leading-snug tracking-tight">
              {t.title}
            </h2>

            {#if t.body}
              <p class="line-clamp-1 text-sm text-muted-foreground">{t.body}</p>
            {/if}

            <div class="mt-1 flex items-center gap-1.5 text-xs text-muted-foreground">
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
    <LoadMore loading={loadingMore} error={loadFailed} onclick={loadMore} />
  {/if}
{/if}
