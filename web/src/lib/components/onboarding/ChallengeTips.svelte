<script lang="ts">
  // "What helps": what on freehire answers each thing the visitor said is in their way
  // (the table lives in #lib/onboardingChallenge). Nothing renders when nothing was picked.
  import {
    ArrowRight,
    Building2,
    FilePenLine,
    Ghost,
    Globe,
    Handshake,
    Languages,
    Lightbulb,
    ListFilter,
    MessagesSquare,
    Plane,
    Puzzle,
    SlidersHorizontal,
    SquareKanban,
    Sprout,
    Stamp,
    Target,
    Users,
  } from '@lucide/svelte';
  import { challengeTips, type TipIcon } from '#lib/onboardingChallenge.js';
  import { Badge } from '#lib/ui/index.js';
  import FlowStep from './FlowStep.svelte';

  let { picked }: { picked: string[] } = $props();

  const TIP_ICONS: Record<TipIcon, typeof Target> = {
    tailor: FilePenLine,
    extension: Puzzle,
    referral: Handshake,
    tracking: SquareKanban,
    feed: ListFilter,
    skills: Target,
    ghost: Ghost,
    globe: Globe,
    filters: SlidersHorizontal,
    relocation: Plane,
    visa: Stamp,
    companies: Building2,
    buddies: Users,
    mentors: MessagesSquare,
    language: Languages,
    junior: Sprout,
  };

  const tips = $derived(challengeTips(picked));
</script>

{#if tips.length > 0}
  <FlowStep icon={Lightbulb} title="What helps" subtitle="For what you said is in your way." state="done">
    <ul class="-mx-1 divide-y divide-border">
      {#each tips as t (t.title)}
        {@const TipGlyph = TIP_ICONS[t.icon]}
        <li>
          <!-- eslint-disable svelte/no-navigation-without-resolve -- tips carry a site path with its query; resolve() takes a route, not a URL -->
          <a href={t.href} class="group flex items-start gap-3 rounded-lg px-1 py-3 transition-colors hover:bg-accent">
            <span class="grid size-9 shrink-0 place-items-center rounded-lg bg-brand-muted text-brand-strong">
              <TipGlyph class="size-4.5" aria-hidden="true" />
            </span>
            <span class="min-w-0 flex-1">
              <span class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm font-semibold">
                {t.title}
                {#if t.pro}<Badge variant="brand">Pro</Badge>{/if}
              </span>
              <span class="mt-0.5 block text-xs leading-snug text-muted-foreground text-pretty">{t.body}</span>
            </span>
            <ArrowRight
              class="mt-2.5 size-4 shrink-0 text-muted-foreground transition-colors group-hover:text-brand-strong"
              aria-hidden="true"
            />
          </a><!-- eslint-enable svelte/no-navigation-without-resolve -->
        </li>
      {/each}
    </ul>
  </FlowStep>
{/if}
