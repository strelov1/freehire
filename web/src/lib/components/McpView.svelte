<script lang="ts">
  import { resolve } from '$app/paths';
  import { MCP_CATALOG_PITCH } from '#lib/mcpCopy.js';
  import { SectionLabel } from '#lib/ui/index.js';
  import AgentSetupButton from './AgentSetupButton.svelte';
  import AssistantSetup from './AssistantSetup.svelte';

  // Things to ask, grouped by what the visitor is after. Each one is answerable by
  // public tools alone, except the last group, which needs OAuth on the same connection.
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
</script>

<!-- Hero: the pitch. -->
<section class="pb-10 pt-12 sm:pt-16">
  <div>
    <SectionLabel text="MCP" />
    <h1 class="mt-6 max-w-3xl text-balance text-4xl font-semibold leading-none tracking-tighter sm:text-6xl">
      Search jobs right inside your AI assistant.
    </h1>
    <p class="mt-7 max-w-xl text-lg leading-relaxed text-muted-foreground">
      {MCP_CATALOG_PITCH} Find your next role inside Claude, ChatGPT or your coding assistant. Connect your freehire account
      with OAuth to search, save your shortlist, track applications and manage alerts.
    </p>
    <AgentSetupButton />
  </div>
</section>

<AssistantSetup />

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
    <li>
      It connects only after you sign in to freehire and approve access, and you can revoke it from Connected devices.
    </li>
    <li>
      With your account it reads your profile and tracked jobs, and saves, marks applied or moves jobs on your board —
      never another person's. It can also manage alerts, suggest missing dictionary terms and report problems. Automatic
      applications require your permission, a supported employer form and review before submission.
    </li>
  </ul>
  <p class="mt-8 text-sm leading-relaxed text-muted-foreground">
    Prefer a local server or the terminal? The <a
      href={resolve('/cli')}
      class="font-medium text-foreground underline-offset-4 hover:underline">CLI and npx MCP server</a
    > use the same API.
  </p>
</section>
