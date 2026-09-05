<script lang="ts">
  import { Check, ArrowRight } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { outstandingOf } from '$lib/accountCompleteness';
  import { ensureAccountSetupLoaded, setupSteps } from '$lib/accountSetup.svelte';

  // "How complete is my account", at the top of the tracking page — the page a bare /my
  // already redirects to, so the card sits where people land rather than on an account
  // home invented to host it.
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
</script>

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
          {:else}
            <a
              href={resolve(step.href)}
              class="group flex items-center gap-2 rounded-lg px-1 py-1.5 text-sm transition-colors hover:bg-accent"
            >
              <span
                class="size-4 shrink-0 rounded-full border border-dashed border-muted-foreground"
                aria-hidden="true"
              ></span>
              <span class="min-w-0 flex-1">{step.label}</span>
              <ArrowRight
                class="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
                aria-hidden="true"
              />
            </a>
          {/if}
        </li>
      {/each}
    </ul>
  </section>
{/if}
