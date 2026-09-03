<script lang="ts">
  import { resolve } from '$app/paths';
  import { ArrowUpRight } from '@lucide/svelte';
  import type { DiscussionSubject, SubjectAbsence } from '$lib/discussionSubject';
  import { companyLogoUrl } from '$lib/logo';
  import { Badge, EntityLogo } from '$lib/ui';

  let {
    subject,
    absence,
    subjectType,
    subjectSlug,
    class: className,
  }: {
    /** Null when the subject could not be read. The discussion still renders — it
     *  outlives its subject — but there is nowhere to send the reader. */
    subject: DiscussionSubject | null;
    /** Why it is absent, when it is. `gone` is about the subject, `unreachable` about
     *  us, and a reader told the first when the second is true is told the vacancy has
     *  disappeared on the strength of our own outage. */
    absence?: SubjectAbsence;
    subjectType: 'company' | 'job';
    subjectSlug: string;
    /** Spacing is the caller's: the company page seats this beside a button, the
     *  others give it a row of its own. */
    class?: string;
  } = $props();

  const thing = $derived(subjectType === 'job' ? 'vacancy' : 'company');
  const absentNote = $derived(
    absence === 'unreachable'
      ? `This ${thing} couldn't be loaded just now`
      : `This ${thing} is no longer listed`,
  );

  const box =
    'flex items-center gap-3 rounded-lg border border-border bg-card px-3.5 py-3 text-left';
</script>

{#snippet mark(name: string)}
  <!-- Keyed on the employer's REAL name, and empty when there is none — the proxy
       resolves by name, so a stand-in or a slug would fetch a mark for a string that is
       not a brand. Empty is the nameless tile, the honest answer here. -->
  <EntityLogo
    {name}
    src={name ? (companyLogoUrl(name) ?? undefined) : undefined}
    shape="square"
    size="md"
    class="shrink-0"
  />
{/snippet}

<!-- The whole card is the way back to the subject, so it replaces the bare crumb it
     grew out of: that crumb was a link with no information in it.

     Only when there IS a subject, though. Linking the absent case would hand the reader
     a card that says the vacancy is gone and then sends them to the page saying so
     again — and on the unreachable branch, to a url that may be perfectly fine but that
     we have just failed to read. A plain box states what we know and stops.

     One `subject` test decides both the element and what goes in it, rather than the
     element here and the text again inside. -->
{#if subject}
  <!-- resolve() inline, not through a derived: the no-navigation-without-resolve rule
       reads the href expression itself, as it does in DiscussionFeed. -->
  <a
    href={subjectType === 'company'
      ? resolve('/companies/[slug]', { slug: subjectSlug })
      : resolve('/jobs/[slug]', { slug: subjectSlug })}
    class={[box, 'transition-colors hover:bg-muted/50', className]}
  >
    {@render mark(subject.company)}
    <div class="min-w-0 flex-1">
      <div class="truncate font-semibold leading-snug">{subject.title}</div>
      <div class="mt-0.5 flex items-center gap-2 text-sm text-muted-foreground">
        <!-- For a company subject the name is already the title above; printing it
             again would just say the same thing twice. -->
        {#if subjectType === 'job'}
          <span class="truncate">{subject.employerLabel}</span>
        {/if}
        {#if subject.closed}
          <Badge variant="outline" class="shrink-0">Closed</Badge>
        {/if}
      </div>
    </div>
    <ArrowUpRight class="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
  </a>
{:else}
  <div class={[box, className]}>
    {@render mark('')}
    <div class="min-w-0 flex-1">
      <div class="truncate font-mono text-sm">{subjectSlug}</div>
      <div class="mt-0.5 text-sm text-muted-foreground">{absentNote}</div>
    </div>
  </div>
{/if}
