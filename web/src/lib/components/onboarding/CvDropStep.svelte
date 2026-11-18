<script lang="ts">
  import { FileUp } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { RESUME_MAX_MB } from '#lib/api.js';
  import { isAuthenticated } from '#lib/auth.svelte.js';
  import { onboardingGate } from '#lib/onboardingGate.svelte.js';
  import { signinUrl } from '#lib/signin.js';
  import { Button } from '#lib/ui/index.js';
  import FlowStep from './FlowStep.svelte';

  // Onboarding's first step: the CV drop (a file picker under a drop target), what went
  // wrong with the last file, and the ways past it for someone without a CV to hand.
  // The page reads the file (`onfile`).
  let {
    dropError,
    limited,
    here,
    returnTo,
    onfile,
    onskip,
  }: {
    dropError: string | null;
    /** The last read hit the anonymous daily limit. */
    limited: boolean;
    /** This page, for a sign-in that should come back to it. */
    here: string;
    /** Where the visitor was headed before onboarding, if anywhere. */
    returnTo: string | null | undefined;
    onfile: (file: File) => void;
    onskip: () => void;
  } = $props();

  let fileInput = $state<HTMLInputElement>();
  let dragActive = $state(false);

  function onPick(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const f = input.files?.[0];
    input.value = ''; // the same file can be picked again after an error
    if (f) onfile(f);
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    dragActive = false;
    const f = e.dataTransfer?.files?.[0];
    if (f) onfile(f);
  }
</script>

<FlowStep
  icon={FileUp}
  title="Start with your CV"
  subtitle="We read it and show you what we found. Nothing is stored until you save it."
>
  <input type="file" accept="application/pdf,.pdf" class="hidden" bind:this={fileInput} onchange={onPick} />
  <!-- A drop target over the button inside, which is fully keyboard-accessible alone. -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    data-testid="onboarding-dropzone"
    ondragover={(e) => {
      e.preventDefault();
      dragActive = true;
    }}
    ondragleave={(e) => {
      e.preventDefault();
      dragActive = false;
    }}
    ondrop={onDrop}
    class={[
      'flex flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed px-4 py-8 text-center transition-colors sm:px-6 sm:py-10',
      dragActive ? 'border-brand bg-brand-muted' : 'border-border',
    ]}
  >
    <p class="text-sm font-medium pointer-coarse:hidden">Drop your CV here, or</p>
    <Button variant="primary" onclick={() => fileInput?.click()}>Choose a PDF</Button>
    <p class="text-xs text-muted-foreground">PDF with selectable text, up to {RESUME_MAX_MB}&nbsp;MB.</p>
  </div>
  {#if dropError}
    <div class="flex flex-col gap-2 text-sm" role="alert">
      <p class="text-destructive">{dropError}</p>
      {#if limited && !isAuthenticated()}
        <p class="text-muted-foreground">Signed in, there is no daily limit — sign in and drop it again.</p>
        <Button variant="outline" size="sm" class="self-start" href={signinUrl({ returnTo: here })}>
          Sign in or create an account
        </Button>
      {/if}
    </div>
  {/if}
  {#if !isAuthenticated()}
    <!-- Someone who already has an account came to sign in, not to be onboarded: their
         profile and CV are already stored, so the flow has nothing to add for them. -->
    <p class="text-sm text-muted-foreground">
      Already have an account?
      <!-- eslint-disable svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query -->
      <a
        href={signinUrl({ returnTo: returnTo ?? resolve('/'), mode: 'login' })}
        onclick={() => onboardingGate.skipAsGuest()}
        class="font-medium text-foreground underline-offset-4 hover:underline">Sign in</a
      ><!-- eslint-enable svelte/no-navigation-without-resolve -->
    </p>
    <button
      type="button"
      onclick={onskip}
      class="self-start text-sm text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
    >
      Skip the CV and create an account
    </button>
  {/if}
</FlowStep>
