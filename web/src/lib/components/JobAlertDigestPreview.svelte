<script lang="ts">
  import { resolve } from '$app/paths';
  import { EntityLogo } from '#lib/ui/index.js';
  import BrandMark from '#lib/components/BrandMark.svelte';

  // The job-alert email in miniature, mirroring the real template — shared by the
  // notifications landing page and the onboarding tour. Decorative: the jobs are
  // illustrative, all named for one saved search so the subject and the rows agree.
  // `bare` drops the outer frame for a surface that already draws its own.
  // `compact` keeps only the subject and the first two rows, fading out — a glimpse of the
  // email rather than the whole of it.
  let {
    search = 'Remote Go Backend',
    bare = false,
    compact = false,
  }: { search?: string; bare?: boolean; compact?: boolean } = $props();

  const digestJobs = [
    { company: 'Grafana', title: 'Platform Engineer' },
    { company: 'Datadog', title: 'Senior Backend Engineer' },
    { company: 'Cloudflare', title: 'Staff Backend Engineer' },
  ];
</script>

{#snippet mail()}
  <div
    class={[
      'rounded-xl border border-border bg-card p-5 shadow-sm',
      compact && 'max-h-56 overflow-hidden [mask-image:linear-gradient(to_bottom,black_60%,transparent)]',
    ]}
  >
    <div class="flex items-center gap-2">
      <BrandMark class="size-5 text-foreground" />
      <span class="text-sm font-bold">freehire</span>
    </div>
    <h3 class="mt-4 text-base font-semibold tracking-tight">3 new jobs for &ldquo;{search}&rdquo;</h3>
    <div class="mt-4 flex flex-col gap-2">
      {#each compact ? digestJobs.slice(0, 2) : digestJobs as job (job.title)}
        <div class="flex items-center gap-3 rounded-lg border border-border p-2.5">
          <EntityLogo name={job.company} shape="square" size="xs" />
          <div class="min-w-0">
            <p class="truncate text-sm font-medium">{job.title}</p>
            <p class="truncate text-xs text-muted-foreground">{job.company}</p>
          </div>
        </div>
      {/each}
    </div>
    {#if !compact}
    <span class="mt-5 inline-block rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-foreground">
      View all — 12 more
    </span>
    <p class="mt-6 border-t border-border pt-4 text-xs text-muted-foreground">
      You're getting this because you set up a job alert on freehire.
      <a href={resolve('/my/notifications/settings')} class="underline-offset-4 hover:underline">Manage notifications</a>.
    </p>
    {/if}
  </div>
{/snippet}

{#if bare}
  {@render mail()}
{:else}
  <figure class="overflow-hidden rounded-xl border border-border bg-secondary/40 shadow-sm">
    <figcaption class="border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
      3 new jobs for "{search}"
    </figcaption>
    <div class="p-4 sm:p-5">
      {@render mail()}
    </div>
  </figure>
{/if}
