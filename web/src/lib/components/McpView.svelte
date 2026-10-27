<script lang="ts">
  import { resolve } from '$app/paths';
  import { SectionLabel } from '#lib/ui/index.js';
  import McpSetup from './McpSetup.svelte';

  // Things to ask, grouped by what the visitor is after. Each one is answerable by
  // the anonymous server's tools alone, except the last group, which needs the account.
  const prompts = [
    {
      title: 'Find jobs',
      items: [
        'Remote senior Go jobs in Europe posted this week',
        'Junior data analyst roles in Berlin with a stated salary',
      ],
    },
    {
      title: 'Look at an employer',
      items: ['What is Stripe hiring for right now?', 'Fintech companies hiring backend engineers in Poland'],
    },
    {
      title: 'Read the market',
      items: ['What does a senior frontend role pay in Germany?', 'Which skills do DevOps postings ask for most?'],
    },
    {
      title: 'With your account',
      items: ['Find jobs that fit my profile and save the best three', 'Which of my applications have gone quiet?'],
    },
  ];

  // The hero's sample conversation. Illustrative, so no real employer is named.
  const heroJobs = [
    { title: 'Senior Go Engineer — payments platform', meta: 'Remote · EU · €85–100k' },
    { title: 'Senior Backend Engineer (Go, Kafka)', meta: 'Remote · Germany, Netherlands' },
    { title: 'Staff Go Developer — observability', meta: 'Remote · Europe · posted 2 days ago' },
  ];
</script>



<!-- Hero. Left: the pitch. Right: what a conversation with the connector looks like. -->
<section class="grid items-center gap-10 pb-10 pt-12 sm:pt-16 lg:grid-cols-2">
  <div>
    <SectionLabel text="mcp" />
    <h1 class="mt-6 max-w-3xl text-balance text-4xl font-semibold leading-none tracking-tighter sm:text-6xl">
      Search jobs right inside your AI assistant.
    </h1>
    <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground">
      Free. No freehire account needed to search — sign in only if you want the assistant to save and track jobs for
      you.
    </p>
  </div>

  <figure class="overflow-hidden rounded-xl border border-border bg-secondary/60 text-sm shadow-sm">
    <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
      <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
      Your assistant · freehire connector
    </figcaption>
    <div class="flex flex-col gap-3 p-4 leading-relaxed">
      <div class="max-w-xs self-end rounded-2xl rounded-br-sm bg-foreground px-3.5 py-2 text-background">
        Remote senior Go jobs in Europe, posted this week
      </div>
      <p class="text-xs text-muted-foreground">Used freehire · Search jobs</p>
      <div class="max-w-md rounded-2xl rounded-bl-sm border border-border bg-background px-3.5 py-2.5">
        <p class="text-muted-foreground">Here are the strongest matches:</p>
        {#each heroJobs as job (job.title)}
          <div class="mt-2.5">
            <p class="font-medium text-foreground">{job.title}</p>
            <p class="text-xs text-muted-foreground">{job.meta}</p>
          </div>
        {/each}
      </div>
      <div class="max-w-xs self-end rounded-2xl rounded-br-sm bg-foreground px-3.5 py-2 text-background">
        Save the first one to my board
      </div>
    </div>
  </figure>
</section>

<McpSetup />

<!-- Things to ask. -->
<section class="border-t border-border py-14 sm:py-16">
  <h2 class="text-2xl font-semibold tracking-tight">Things to ask</h2>
  <div class="mt-6 grid gap-4 sm:grid-cols-2">
    {#each prompts as group (group.title)}
      <div class="rounded-xl border border-border bg-secondary/40 p-4">
        <h3 class="text-sm font-semibold text-foreground">{group.title}</h3>
        <ul class="mt-3 space-y-2">
          {#each group.items as item (item)}
            <li class="text-sm leading-relaxed text-muted-foreground">“{item}”</li>
          {/each}
        </ul>
      </div>
    {/each}
  </div>
</section>

<!-- What it can reach. -->
<section class="border-t border-border py-14 sm:py-16">
  <h2 class="text-2xl font-semibold tracking-tight">What the connector can reach</h2>
  <ul class="mt-6 max-w-2xl list-disc space-y-2 pl-5 leading-relaxed text-muted-foreground">
    <li>Public job postings, employer profiles and market figures — the same catalogue the site shows.</li>
    <li>Without sign-in it cannot see or change anything of yours.</li>
    <li>
      With your account it reads your profile and tracked jobs, and saves, marks applied or moves jobs on your board —
      never another person's.
    </li>
  </ul>
  <p class="mt-8 text-sm leading-relaxed text-muted-foreground">
    Prefer a local server or the terminal? The <a
      href={resolve('/cli')}
      class="font-medium text-foreground underline-offset-4 hover:underline">CLI and npx MCP server</a
    > use the same API.
  </p>
</section>
