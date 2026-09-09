<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { Badge, Card, EmptyState } from '$lib/ui';
  import {
    emptyMentorFilters,
    mentorFiltersToQuery,
    type MentorFilterOptions,
    type MentorFilters,
  } from '$lib/mentorship';
  import type { Mentor } from '$lib/types';

  let {
    mentors,
    options,
    filters,
  }: { mentors: Mentor[]; options: MentorFilterOptions; filters: MentorFilters } = $props();

  // The URL is the state. Each control writes the whole filter set back to the address bar
  // and lets the route's `load` fetch again — the directory is small and hand-onboarded, so
  // there is nothing here that a debounce or a client-side cache would earn.
  //
  // A real navigation, unlike the booker's shallow `replaceState`: the narrowed list comes
  // from the server, so `load` has to run.
  function apply(next: MentorFilters) {
    const query = mentorFiltersToQuery(next);
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() applied to the /mentors path; the rule can't see through the appended query
    void goto(`${resolve('/mentors')}${query ? `?${query}` : ''}`, {
      keepFocus: true,
      noScroll: true,
    });
  }

  const active = $derived(Boolean(filters.company || filters.topic || filters.language));

  // A <select> whose value names no <option> renders BLANK — it does not fall back to the
  // first entry, and the control then lies about what the page is showing. That happens
  // for real here: a link carrying `?company=acme` outlives the last Acme mentor pausing.
  // So a selected value that the options no longer carry is offered as its own option.
  function withSelected<T extends { value: string; label: string }>(
    list: T[],
    selected: string,
  ): { value: string; label: string }[] {
    if (!selected || list.some((o) => o.value === selected)) return list;
    return [{ value: selected, label: selected }, ...list];
  }

  const companyOptions = $derived(
    withSelected(
      options.companies.map((c) => ({ value: c.slug, label: c.name })),
      filters.company,
    ),
  );
  const topicOptions = $derived(
    withSelected(
      options.topics.map((t) => ({ value: t, label: t })),
      filters.topic,
    ),
  );
  const languageOptions = $derived(
    withSelected(
      options.languages.map((l) => ({ value: l, label: l })),
      filters.language,
    ),
  );
</script>

<div class="flex flex-col gap-6">
  <div class="flex flex-wrap items-end gap-3">
    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">Company</span>
      <select
        class="border-input bg-background h-9 rounded-md border px-2 text-sm"
        value={filters.company}
        onchange={(e) => apply({ ...filters, company: e.currentTarget.value })}
      >
        <option value="">Any company</option>
        {#each companyOptions as option (option.value)}
          <option value={option.value}>{option.label}</option>
        {/each}
      </select>
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">Topic</span>
      <select
        class="border-input bg-background h-9 rounded-md border px-2 text-sm"
        value={filters.topic}
        onchange={(e) => apply({ ...filters, topic: e.currentTarget.value })}
      >
        <option value="">Any topic</option>
        {#each topicOptions as option (option.value)}
          <option value={option.value}>{option.label}</option>
        {/each}
      </select>
    </label>

    <label class="flex flex-col gap-1 text-sm">
      <span class="text-muted-foreground">Language</span>
      <select
        class="border-input bg-background h-9 rounded-md border px-2 text-sm"
        value={filters.language}
        onchange={(e) => apply({ ...filters, language: e.currentTarget.value })}
      >
        <option value="">Any language</option>
        {#each languageOptions as option (option.value)}
          <option value={option.value}>{option.label}</option>
        {/each}
      </select>
    </label>

    {#if active}
      <button
        type="button"
        class="text-muted-foreground hover:text-foreground h-9 text-sm underline"
        onclick={() => apply(emptyMentorFilters())}
      >
        Clear filters
      </button>
    {/if}
  </div>

  {#if mentors.length === 0}
    <EmptyState
      title={active ? 'No mentors match these filters' : 'No mentors yet'}
      description={active
        ? 'Try a wider search — clear a filter and see who else is available.'
        : 'Mentors are onboarded by hand. Check back soon.'}
    />
  {:else}
    <ul class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {#each mentors as mentor (mentor.slug)}
        <li>
          <a
            href={resolve('/mentors/[slug]', { slug: mentor.slug })}
            class="focus-visible:ring-ring block rounded-lg focus-visible:ring-2 focus-visible:outline-none"
          >
            <Card class="h-full p-4">
              <div class="flex flex-col gap-2">
                <div class="flex items-center gap-3">
                  {#if mentor.show_photo}
                    <img
                      src={`/api/v1/mentors/${mentor.slug}/photo`}
                      alt=""
                      loading="lazy"
                      class="border-border size-10 shrink-0 rounded-full border object-cover"
                      onerror={(e) => {
                        (e.currentTarget as HTMLImageElement).style.display = 'none';
                      }}
                    />
                  {/if}
                  <div>
                    <p class="font-medium">{mentor.name}</p>
                    <p class="text-muted-foreground text-sm">
                      {mentor.headline} · {mentor.company_name}
                    </p>
                  </div>
                </div>

                {#if mentor.topics.length > 0}
                  <div class="flex flex-wrap gap-1">
                    {#each mentor.topics as topic (topic)}
                      <Badge variant="secondary">{topic}</Badge>
                    {/each}
                  </div>
                {/if}

                <p class="text-muted-foreground text-sm">
                  {mentor.session_minutes}-minute session
                  {#if mentor.rating_count > 0}
                    · {mentor.rating_avg.toFixed(1)} ★ ({mentor.rating_count})
                  {/if}
                </p>
              </div>
            </Card>
          </a>
        </li>
      {/each}
    </ul>
  {/if}
</div>
