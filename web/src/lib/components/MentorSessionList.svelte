<script lang="ts">
  import { resolve } from '$app/paths';
  import { Badge, Card, EmptyState } from '$lib/ui';
  import { formatInstantIn } from '$lib/mentorship';
  import type { MentorSession } from '$lib/types';

  let {
    sessions,
    timezone,
    empty,
  }: { sessions: MentorSession[]; timezone: string; empty: string } = $props();
</script>

{#if sessions.length === 0}
  <EmptyState title={empty} />
{:else}
  <ul class="flex flex-col gap-3">
    {#each sessions as session (session.id)}
      {@const when = formatInstantIn(session.starts_at, timezone)}
      <li>
        <a
          href={resolve('/my/mentorship/sessions/[id]', { id: session.id })}
          class="focus-visible:ring-ring block rounded-lg focus-visible:ring-2 focus-visible:outline-none"
        >
          <Card class="p-4">
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <div>
                <p class="font-medium">{when.day} at {when.time}</p>
                <p class="text-muted-foreground text-sm">
                  <!-- Whichever of the two the row knows. A seeker's list carries the
                       mentor's headline; a MENTOR's list carries the seeker's address
                       instead, and says so — the endpoint sends it precisely so this pane
                       can name who is coming, and the mentor is meeting this person. -->
                  {session.seeker_email || session.headline}
                  <!-- The offset, for the same reason a slot carries it: on the autumn
                       transition two sessions an hour apart read as the same clock time. -->
                  · UTC{when.offset}
                </p>
              </div>
              {#if session.status !== 'confirmed'}
                <Badge variant="secondary">{session.status}</Badge>
              {:else if session.completed}
                <Badge variant="outline">completed</Badge>
              {/if}
            </div>
          </Card>
        </a>
      </li>
    {/each}
  </ul>
{/if}
