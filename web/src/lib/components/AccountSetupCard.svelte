<script lang="ts">
  import { Check, ArrowRight } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { outstandingOf, stepLeadsSomewhere, type CompletenessStep } from '$lib/accountCompleteness';
  import { ensureAccountSetupLoaded, setupSteps } from '$lib/accountSetup.svelte';

  // "How complete is my account", mounted in /my/profile's LAYOUT — so it is on screen for
  // all eight profile sections, and four of its five steps are done on one of them (only
  // the alert lives elsewhere). That is why a step is rendered as a link only when
  // `stepLeadsSomewhere` says following it would move the reader.
  //
  // A funnel beats a choose-your-own-adventure: the onboarding wizard asks these same
  // questions once, and this is what remains of them afterwards for anyone who skipped a
  // step or signed up before the wizard existed. It disappears the moment nothing is
  // outstanding — a card that congratulates you forever is a permanent tax on the page.

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
  <span
    class="size-4 shrink-0 rounded-full border border-dashed border-muted-foreground"
    aria-hidden="true"
  ></span>
  <span class="min-w-0 flex-1">{step.label}</span>
{/snippet}

{#if outstanding.length > 0}
  <!-- No outer margin: every host so far is a flex column that owns its own spacing,
       and a margin here would add to that gap rather than replace it. -->
  <section class="rounded-xl border border-border bg-card p-4" aria-labelledby="account-setup-heading">
    <div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
      <h2 id="account-setup-heading" class="text-sm font-semibold tracking-tight">
        Finish setting up your account
      </h2>
      <!-- The count, not a percentage bar: five steps is few enough that "3 of 5" is the
           more precise statement and needs no legend. `tabular-nums` keeps it from
           twitching as steps are completed. -->
      <p class="text-xs tabular-nums text-muted-foreground">{done} of {steps.length} done</p>
    </div>

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
              <span class="line-through decoration-border">{step.label}</span>
            </p>
          {:else if leadsSomewhere(step)}
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- stepHref() wraps resolve(step.href); the rule can't see through the appended #anchorId -->
            <a href={stepHref(step)} class="group flex items-center gap-2 rounded-lg px-1 py-1.5 text-sm transition-colors hover:bg-accent">
              {@render dotAndLabel(step)}
              <ArrowRight
                class="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
                aria-hidden="true"
              />
            </a>
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
  </section>
{/if}
