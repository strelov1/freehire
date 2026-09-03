<script lang="ts">
  import { resolve } from '$app/paths';
  import { ArrowUpRight } from '@lucide/svelte';
  import type { DiscussionSubject } from '$lib/discussionSubject';
  import { companyLogoUrl } from '$lib/logo';
  import { Badge, EntityLogo } from '$lib/ui';

  let {
    subject,
    subjectType,
    subjectSlug,
    class: className,
  }: {
    /** Null when the subject could not be read — pruned, or the API was unreachable.
     *  The header then names the slug, because a discussion outlives its subject and
     *  must stay readable. */
    subject: DiscussionSubject | null;
    subjectType: 'company' | 'job';
    subjectSlug: string;
    /** Spacing is the caller's: the company page seats this beside a button, the
     *  others give it a row of its own. */
    class?: string;
  } = $props();
</script>

<!-- The whole card is the way back to the subject, so it replaces the bare crumb it
     grew out of: that crumb was a link with no information in it. -->
<a
  class={[
    'flex items-center gap-3 rounded-lg border border-border bg-card px-3.5 py-3 transition-colors hover:bg-muted/50',
    className,
  ]}
  href={subjectType === 'company'
    ? resolve('/companies/[slug]', { slug: subjectSlug })
    : resolve('/jobs/[slug]', { slug: subjectSlug })}
>
  <!-- Keyed on the employer name, and empty when there is none — the proxy resolves by
       name, so a slug would fetch a mark for a string that is not a brand. -->
  <EntityLogo
    name={subject?.company ?? ''}
    src={subject?.company ? (companyLogoUrl(subject.company) ?? undefined) : undefined}
    shape="square"
    size="md"
    class="shrink-0"
  />

  <div class="min-w-0 flex-1">
    {#if subject}
      <div class="truncate font-semibold leading-snug">{subject.title}</div>
      <div class="mt-0.5 flex items-center gap-2 text-sm text-muted-foreground">
        <!-- For a company subject the name is already the title above; printing it
             again would just say the same thing twice. -->
        {#if subject.kind === 'job'}
          <span class="truncate">{subject.company}</span>
        {/if}
        {#if subject.closed}
          <Badge variant="outline" class="shrink-0">Closed</Badge>
        {/if}
      </div>
    {:else}
      <div class="truncate font-mono text-sm" title="This subject could not be loaded">
        {subjectSlug}
      </div>
      <div class="mt-0.5 text-sm text-muted-foreground">
        {subjectType === 'job' ? 'Vacancy' : 'Company'} unavailable
      </div>
    {/if}
  </div>

  <ArrowUpRight class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
</a>
