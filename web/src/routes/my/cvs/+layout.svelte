<script lang="ts">
  import type { Snippet } from 'svelte';
  import { FileText, LayoutTemplate, Type } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { Button, TabStrip, tabStripId } from '$lib/ui';
  import { activeRouteTab } from '$lib/routeTabs';
  import { cvIntakeDialog, closeCvIntake, openCvIntake } from '$lib/cvIntakeDialog.svelte';
  import JdIntakeDialog from '$lib/components/cv/JdIntakeDialog.svelte';

  let { children }: { children: Snippet } = $props();

  // The account shell (my/+layout) owns the container, auth gate, and noindex; this
  // layout adds the CV section's own navigation — the same underline TabStrip every other
  // account section navigates with. The appearance defaults used to be one "Settings"
  // page reached by a button; they are the section's other two views, and the record
  // behind them is shared (see cvAppearance.svelte.ts), so a tab switch keeps an
  // unsaved edit.
  //
  // Starting a tailored CV is an action on the section, not one of its views, so it
  // stays a button beside the strip — and stays reachable from every tab, which is why
  // the dialog is mounted here rather than inside the list.
  const SECTIONS = [
    { id: 'list', label: 'List', href: '/my/cvs', icon: FileText },
    { id: 'template', label: 'Template', href: '/my/cvs/template', icon: LayoutTemplate },
    { id: 'typography', label: 'Typography', href: '/my/cvs/typography', icon: Type },
  ] as const;
  const PANEL_ID = 'cvs-panel';

  const active = $derived(activeRouteTab(page.url.pathname, SECTIONS, 'list'));
  const tabs = $derived(SECTIONS.map((sec) => ({ ...sec, href: resolve(sec.href) })));
</script>

<div class="flex max-w-3xl flex-col gap-6">
  <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
    <div class="flex flex-col gap-1">
      <h1 class="text-2xl font-semibold tracking-tight">Tailored CVs</h1>
      <p class="text-sm text-muted-foreground">
        CVs you tailored for specific roles, and the appearance a new one starts with.
      </p>
    </div>
    <div class="shrink-0">
      <Button variant="outline" onclick={openCvIntake}>Tailor for a job</Button>
    </div>
  </div>

  <TabStrip {tabs} {active} label="CV sections" panelId={PANEL_ID} />

  <div role="tabpanel" id={PANEL_ID} aria-labelledby={tabStripId(PANEL_ID, active)} tabindex="0">
    {@render children()}
  </div>
</div>

{#if cvIntakeDialog.open}
  <JdIntakeDialog onClose={closeCvIntake} />
{/if}
