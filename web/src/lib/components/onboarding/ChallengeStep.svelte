<script lang="ts">
  // "What's hardest about your job search?" — several answers, and beside them what on
  // freehire helps with each (the table lives in onboardingSteps.ts).
  //
  // The free-text box appears only while "Something else" is picked, because the server
  // accepts a note only then: a note beside coded answers alone could contradict them.
  import {
    ArrowRight,
    Building2,
    Check,
    FilePenLine,
    Flag,
    Ghost,
    Globe,
    Handshake,
    Languages,
    Laptop,
    ListFilter,
    MessageSquareReply,
    MessagesSquare,
    PenLine,
    Plane,
    Puzzle,
    SlidersHorizontal,
    SquareKanban,
    Sprout,
    Stamp,
    Target,
    Users,
  } from '@lucide/svelte';
  import { challengeTips, toggleChallenge, type TipIcon } from '#lib/onboardingSteps.js';
  import { JOB_CHALLENGE_OPTIONS, JOB_CHALLENGE_OTHER } from '#lib/surveyOptions.js';
  import { Badge } from '#lib/ui/index.js';
  import SproutIllustration from './SproutIllustration.svelte';

  interface Props {
    value: string[];
    note: string;
    onChange: (next: string[]) => void;
    onNoteChange: (note: string) => void;
  }

  let { value, note, onChange, onNoteChange }: Props = $props();

  type Icon = typeof PenLine;

  const ANSWER_ICONS: Record<string, Icon> = {
    recruiter_contact: MessageSquareReply,
    job_fit: Target,
    remote_flexible: Laptop,
    working_abroad: Plane,
    visa_sponsorship: Stamp,
    technical_interviews: Users,
    english: Languages,
    first_job: Sprout,
    just_starting: Flag,
    other: PenLine,
  };

  const TIP_ICONS: Record<TipIcon, Icon> = {
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

  const tips = $derived(challengeTips(value));

  let panel = $state<HTMLElement>();

  // On a phone the panel sits under ten answers, out of sight; bring it up after a pick.
  function pick(answer: string) {
    const adding = !value.includes(answer);
    onChange(toggleChallenge(value, answer));
    if (adding && panel && !window.matchMedia('(min-width: 768px)').matches) {
      requestAnimationFrame(() => panel?.scrollIntoView({ behavior: 'smooth', block: 'start' }));
    }
  }
</script>

<h2 class="text-xl font-semibold tracking-tight text-balance">What's hardest about your job search right now?</h2>
<p class="mt-1 text-sm text-muted-foreground text-pretty">Pick all that apply — we'll show you what on freehire helps.</p>

<div class="mt-6 grid items-start gap-5 md:grid-cols-2">
  <div role="group" aria-label="The hardest parts of your job search" class="flex flex-col gap-1.5">
    {#each JOB_CHALLENGE_OPTIONS as o (o.value)}
      {@const on = value.includes(o.value)}
      {@const AnswerIcon = ANSWER_ICONS[o.value] ?? PenLine}
      <button
        type="button"
        aria-pressed={on}
        onclick={() => pick(o.value)}
        class={[
          'flex items-center gap-3 rounded-lg border px-3 py-2.5 text-left text-sm transition-colors',
          on ? 'border-brand bg-brand-muted/50 font-medium' : 'border-border bg-card hover:bg-accent',
        ]}
      >
        <AnswerIcon class={['size-4 shrink-0', on ? 'text-brand-strong' : 'text-muted-foreground']} aria-hidden="true" />
        <span class="flex-1">{o.label}</span>
        {#if on}<Check class="size-4 shrink-0 text-brand-strong" aria-hidden="true" />{/if}
      </button>
    {/each}
  </div>

  <section
    bind:this={panel}
    aria-live="polite"
    aria-label="What helps"
    class="scroll-mt-4 rounded-xl border border-border bg-secondary/40 px-4 pb-2 pt-4"
  >
    <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">What helps</p>
    {#if tips.length === 0}
      <div class="flex flex-col items-center px-4 pb-6 pt-4 text-center">
        <SproutIllustration />
        <p class="mt-2 text-sm font-medium">Every search gets stuck somewhere</p>
        <p class="mt-1 max-w-56 text-xs text-muted-foreground text-pretty">
          Pick what's in your way and we'll show you what helps.
        </p>
      </div>
    {:else}
      <ul class="-mx-1 mt-2 divide-y divide-border">
        {#each tips as t (t.title)}
          {@const TipGlyph = TIP_ICONS[t.icon]}
          <li>
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- tips carry a site path with its query; resolve() takes a route, not a URL -->
            <a href={t.href} target="_blank" class="group flex items-start gap-3 rounded-lg px-1 py-3 transition-colors hover:bg-card">
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
            </a>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  {#if value.includes(JOB_CHALLENGE_OTHER)}
    <textarea
      value={note}
      oninput={(e) => onNoteChange(e.currentTarget.value)}
      rows="3"
      aria-label="Tell us in your own words"
      placeholder="Something else? Tell us in your own words"
      class="w-full rounded-lg border border-input bg-card p-3 text-sm outline-none placeholder:text-muted-foreground focus:ring-2 focus:ring-ring md:col-span-2"
    ></textarea>
  {/if}
</div>
