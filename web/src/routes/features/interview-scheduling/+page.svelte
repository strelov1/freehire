<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import {
    ArrowDown,
    ArrowRight,
    CalendarCheck,
    CalendarDays,
    Mail,
    MailCheck,
    MessagesSquare,
    Video,
  } from '@lucide/svelte';
  import Seo from '#lib/components/Seo.svelte';
  import { signinUrl } from '#lib/signin.js';
  import { Badge, Button, ProviderIcon } from '#lib/ui/index.js';

  const path = resolve('/features/interview-scheduling');
  const integrations = resolve('/my/integrations');
  const signedIn = $derived(!!page.data.user);
  const setupHref = $derived(
    signedIn ? integrations : signinUrl({ returnTo: integrations, cancelTo: path, mode: 'login' }),
  );
  const steps = [
    {
      icon: MailCheck,
      title: 'Find the right invitation',
      text: 'The agent starts when it recognizes an interview invitation and can link the email to a single tracked application.',
    },
    {
      icon: MessagesSquare,
      title: 'Offer times in the same thread',
      text: 'The agent checks your availability in Google Calendar and replies from your Gmail with open times. It hands unclear replies back to you.',
    },
    {
      icon: CalendarCheck,
      title: 'Book the agreed time',
      text: 'Once the recruiter clearly picks a time, the agent books it in Google Calendar, sends a calendar invite with Google Meet, and confirms by email.',
    },
  ];
</script>

<Seo
  title="Interview scheduling agent | freehire"
  description="In a limited Freehire pilot, the interview scheduling agent proposes times in Gmail and books the agreed slot in Google Calendar with Google Meet."
  canonical={page.url.origin + path}
/>

{#snippet setupButton()}
  <Button href={setupHref} variant="primary" size="lg">
    {signedIn ? 'Open Integrations' : 'Sign in to check access'}
    <ArrowRight class="size-4" aria-hidden="true" />
  </Button>
{/snippet}

<div class="mx-auto w-full max-w-6xl px-4">
  <section class="grid items-center gap-10 py-12 sm:py-16 lg:grid-cols-2 lg:gap-12 lg:py-20">
    <div>
      <div class="flex flex-wrap items-center gap-3">
        <span class="text-sm font-medium text-muted-foreground">Interview scheduling agent</span>
        <Badge variant="brand">Limited pilot</Badge>
      </div>
      <h1 class="mt-5 max-w-xl text-balance text-4xl leading-none font-semibold tracking-tighter sm:text-6xl">
        Turn recruiter emails into booked interviews.
      </h1>
      <p class="mt-5 max-w-lg text-lg leading-relaxed text-muted-foreground">
        Freehire offers times from your calendar in the recruiter’s email thread and books the interview once the
        recruiter picks a slot.
      </p>
      <div class="mt-7 flex flex-wrap items-center gap-x-3 gap-y-2">
        {@render setupButton()}
        <Button href="#how-it-works" variant="ghost">How it works</Button>
      </div>
      <p class="mt-4 text-sm text-muted-foreground">Available to eligible Pro and Ultra accounts during the pilot.</p>
    </div>

    <figure class="rounded-2xl border border-border bg-secondary/50 p-4 sm:p-6">
      <figcaption class="mb-4 flex items-center justify-between gap-3 text-sm">
        <span class="font-semibold">From email to calendar</span>
        <span class="text-muted-foreground">Example</span>
      </figcaption>
      <div class="rounded-xl border border-border bg-card px-4 shadow-sm sm:px-5">
        <div class="flex items-start gap-3 py-4">
          <Mail class="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          <div>
            <p class="text-sm font-semibold">Recruiter</p>
            <p class="mt-1 text-sm text-muted-foreground">Could we find a time for an interview?</p>
          </div>
        </div>
        <div class="flex items-start gap-3 border-t border-border py-4">
          <MessagesSquare class="mt-0.5 size-4 shrink-0 text-brand-strong" aria-hidden="true" />
          <div>
            <p class="text-sm font-semibold">Freehire replies in Gmail</p>
            <p class="mt-1 text-sm text-muted-foreground">Here are three times that work.</p>
            <ul class="mt-2.5 flex flex-wrap gap-1.5 text-xs">
              <li class="rounded-md bg-secondary px-2 py-1">Tue 11:30</li>
              <li class="rounded-md bg-secondary px-2 py-1">Wed 15:30</li>
              <li class="rounded-md bg-secondary px-2 py-1">Thu 13:30</li>
            </ul>
            <p class="mt-1.5 text-xs text-muted-foreground">All times UTC</p>
          </div>
        </div>
        <div class="flex items-start gap-3 border-t border-border py-4">
          <Mail class="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          <div>
            <p class="text-sm font-semibold">Recruiter</p>
            <p class="mt-1 text-sm text-muted-foreground">Wednesday at 15:30 works for me.</p>
          </div>
        </div>
      </div>
      <ArrowDown class="mx-auto my-2.5 size-4 text-muted-foreground" aria-hidden="true" />
      <div
        class="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl border border-brand-ring/30 bg-brand-muted p-4 text-brand-strong"
      >
        <CalendarCheck class="size-5 shrink-0" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <p class="text-sm font-semibold">Interview booked</p>
          <p class="mt-1 text-xs">Wed 15:30 UTC · Invite and confirmation sent</p>
        </div>
        <span class="flex items-center gap-1.5 text-xs font-medium">
          <Video class="size-4" aria-hidden="true" /> Google Meet
        </span>
      </div>
    </figure>
  </section>

  <section id="how-it-works" class="scroll-mt-20 border-t border-border py-12 sm:py-16">
    <h2 class="text-3xl font-semibold tracking-tight sm:text-4xl">How it works</h2>
    <ol class="mt-7 grid gap-px overflow-hidden rounded-xl border border-border bg-border md:grid-cols-3">
      {#each steps as step (step.title)}
        <li class="bg-card p-5 sm:p-6">
          <step.icon class="size-5 text-brand-strong" aria-hidden="true" />
          <h3 class="mt-4 text-lg font-semibold tracking-tight">{step.title}</h3>
          <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{step.text}</p>
        </li>
      {/each}
    </ol>
    <div class="mt-5 flex items-start gap-3 rounded-xl border border-border bg-secondary/40 p-5 sm:p-6">
      <Mail class="mt-0.5 size-5 shrink-0 text-brand-strong" aria-hidden="true" />
      <div>
        <h3 class="font-semibold">A daily summary in your inbox</h3>
        <p class="mt-1 text-sm leading-relaxed text-muted-foreground">
          The daily digest brings together the agent’s replies, booked interviews, and conversations that need your
          attention.
        </p>
      </div>
    </div>
  </section>

  <section class="border-t border-border py-12 sm:py-16">
    <div class="grid gap-8 rounded-2xl border border-border bg-card p-5 sm:p-8 lg:grid-cols-2 lg:items-center">
      <div>
        <h2 class="text-3xl font-semibold tracking-tight">Set up interview scheduling</h2>
        <p class="mt-3 max-w-lg text-sm leading-relaxed text-muted-foreground">
          If your account has access, open Integrations and choose
          <strong class="text-foreground">Turn on with Google</strong>. Allow access to Gmail and Google Calendar to
          enable the agent. You can turn it off from the same settings.
        </p>
        <div class="mt-5">
          {@render setupButton()}
        </div>
      </div>
      <dl class="divide-y divide-border rounded-xl border border-border bg-background px-4">
        <div class="flex items-start gap-3 py-4">
          <ProviderIcon provider="google" class="mt-0.5 size-4 shrink-0" />
          <div>
            <dt class="text-sm font-medium">Gmail</dt>
            <dd class="mt-1 text-sm text-muted-foreground">
              Find interview invitations and reply in the original thread.
            </dd>
          </div>
        </div>
        <div class="flex items-start gap-3 py-4">
          <CalendarDays class="mt-0.5 size-4 shrink-0 text-brand-strong" aria-hidden="true" />
          <div>
            <dt class="text-sm font-medium">Google Calendar</dt>
            <dd class="mt-1 text-sm text-muted-foreground">Check free time and create an event with Google Meet.</dd>
          </div>
        </div>
      </dl>
    </div>
  </section>
</div>
