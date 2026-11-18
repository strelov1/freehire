<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Check, ArrowRight, ChevronDown, X } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { outstandingOf, stepLeadsSomewhere, type CompletenessStep } from '#lib/accountCompleteness.js';
  import { dismissAlertsStep, ensureAccountSetupLoaded, setupSteps } from '#lib/accountSetup.svelte.js';

  // "How complete is my account", mounted in /my/profile's LAYOUT — so it is on screen for
  // all eight profile sections, and four of its five steps are done on one of them (only
  // the alert lives elsewhere). That is why a step is rendered as a link only when
  // `stepLeadsSomewhere` says following it would move the reader.
  //
  // A funnel beats a choose-your-own-adventure: the onboarding wizard asks these same
  // questions once, and this is what remains of them afterwards for anyone who skipped a
  // step or signed up before the wizard existed. It disappears the moment nothing is
  // outstanding — a card that congratulates you forever is a permanent tax on the page.

  // `extra` rides along with the checklist: inside the folded card while steps remain
  // (the profile page keeps its Talent Network invite there), on its own once they don't.
  let { extra }: { extra?: Snippet } = $props();

  const steps = $derived(setupSteps());
  const outstanding = $derived(outstandingOf(steps));
  const done = $derived(steps.length - outstanding.length);

  // The signed-in check lives inside ensureAccountSetupLoaded, so both callers share it.
  $effect(() => {
    ensureAccountSetupLoaded();
  });

  // resolve()'s own base plus the step's anchor, when it names one — there is no dynamic
  // route segment to resolve, so this is a plain suffix rather than a second resolve().
  function stepHref(step: CompletenessStep): string {
    return `${resolve(step.href)}${step.anchorId ? `#${step.anchorId}` : ''}`;
  }

  // The rule itself lives beside the steps (accountCompleteness); this only feeds it the
  // two paths, which are the part that needs the router.
  function leadsSomewhere(step: CompletenessStep): boolean {
    return stepLeadsSomewhere(step, resolve(step.href), page.url.pathname);
  }
</script>

<!-- The dashed dot and the label — everything an outstanding step shows whether or not it
     is a link, so the two branches below cannot drift apart in padding or wording. -->
{#snippet dotAndLabel(step: CompletenessStep)}
  <span class="size-4 shrink-0 rounded-full border border-dashed border-muted-foreground" aria-hidden="true"></span>
  <span class="min-w-0 flex-1">{step.label}</span>
{/snippet}

{#if outstanding.length > 0}
  <!-- One line until opened: the count and the next step, so the page's own content is the
       first screen. A native <details>, so the fold needs no script and is a disclosure
       to assistive technology. No outer margin: every host is a flex column that owns its
       spacing. -->
  <details class="group/setup rounded-xl border border-border bg-card" aria-labelledby="account-setup-heading">
    <summary
      class="flex cursor-pointer list-none items-center gap-3 px-4 py-3 text-sm [&::-webkit-details-marker]:hidden"
    >
      <span id="account-setup-heading" class="font-semibold tracking-tight">Finish setting up</span>
      <span class="tabular-nums text-muted-foreground">{done} of {steps.length} done</span>
      <span class="hidden min-w-0 flex-1 truncate text-muted-foreground sm:block">
        · Next: {outstanding[0]?.label}
      </span>
      <ChevronDown
        class="ml-auto size-4 shrink-0 text-muted-foreground transition-transform group-open/setup:rotate-180"
        aria-hidden="true"
      />
    </summary>
    <div class="border-t border-border px-4 pb-4">
      <ul class="mt-3 flex flex-col gap-1">
        {#each steps as step (step.id)}
          <li>
            {#if step.done}
              <!-- Completed steps stay listed rather than disappearing: a list that only
                 shows what is left cannot show progress, and progress is the reason
                 somebody finishes the next one. Not a link — there is nothing to go
                 and do. -->
              <p class="flex items-center gap-2 px-1 py-1.5 text-sm text-muted-foreground">
                <Check class="size-4 shrink-0 text-brand" aria-hidden="true" />
                <!-- Muted with a check, not struck through: a line through it read as cancelled. -->
                <span>{step.label}</span>
              </p>
            {:else if leadsSomewhere(step)}
              <!-- The dismiss button (alerts only) must not nest inside the link, so the
                 row itself carries the hover background and the group, and the link
                 covers only the part that navigates. -->
              <div
                class="group flex items-center gap-1 rounded-lg py-1.5 pl-1 pr-1.5 text-sm transition-colors hover:bg-accent"
              >
                <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- stepHref() wraps resolve(step.href); the rule can't see through the appended #anchorId -->
                <a href={stepHref(step)} class="flex min-w-0 flex-1 items-center gap-2">
                  {@render dotAndLabel(step)}
                  <ArrowRight
                    class="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
                    aria-hidden="true"
                  />
                </a>
                {#if step.id === 'alerts'}
                  <!-- Not everyone is job-hunting right now — this is the only step that
                     can be waved off instead of completed. -->
                  <button
                    type="button"
                    onclick={dismissAlertsStep}
                    aria-label="Hide the &quot;{step.label}&quot; step"
                    class="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-background hover:text-accent-foreground"
                  >
                    <X class="size-4" aria-hidden="true" />
                  </button>
                {:else}
                  <!-- Holds the dismiss button's width, so every row's arrow lines up. -->
                  <span class="size-6 shrink-0" aria-hidden="true"></span>
                {/if}
              </div>
            {:else}
              <!-- The step is done on the page already open. Still listed — it is genuinely
                 outstanding — but as a statement rather than a link, with the section it
                 names sitting right below this card. -->
              <p class="flex items-center gap-2 px-1 py-1.5 text-sm">
                {@render dotAndLabel(step)}
                <span class="shrink-0 text-xs text-muted-foreground">on this page</span>
              </p>
            {/if}
          </li>
        {/each}
      </ul>
      {@render extra?.()}
    </div>
  </details>
{:else}
  {@render extra?.()}
{/if}
