<script lang="ts">
  import AccountPageHeading from '#lib/components/AccountPageHeading.svelte';
  import type { Snippet } from 'svelte';
  import { FileText, LayoutTemplate, Type } from '@lucide/svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { Button, TabStrip, tabStripId } from '#lib/ui/index.js';
  import { activeRouteTab } from '#lib/routeTabs.js';
  import { cvIntakeDialog, closeCvIntake, openCvIntake } from '#lib/cvIntakeDialog.svelte.js';
  import JdIntakeDialog from '#lib/components/cv/JdIntakeDialog.svelte';
  import { messages } from './messages';
  import { locale } from '#lib/i18n/currentLocale.svelte.js';
  import { t } from '#lib/i18n/t.js';

  let { children }: { children: Snippet } = $props();

  const s = $derived(t(messages, locale()));

  // The account shell (my/+layout) owns the container, auth gate, and noindex; this
  // layout adds the CV section's own navigation — the same underline TabStrip every other
  // account section navigates with. The appearance defaults used to be one "Settings"
  // page reached by a button; they are the section's other two views, and the record
  // behind them is shared (see cvAppearance.svelte.ts), so a tab switch keeps an
  // unsaved edit.
  //
  // The layout owns the intake dialog for section-wide and base-specific actions.
  const SECTIONS = $derived([
    { id: 'list', label: s.sections.list, href: '/my/cvs', icon: FileText },
    { id: 'template', label: s.sections.template, href: '/my/cvs/template', icon: LayoutTemplate },
    { id: 'typography', label: s.sections.typography, href: '/my/cvs/typography', icon: Type },
  ] as const);
  const PANEL_ID = 'cvs-panel';

  const active = $derived(activeRouteTab(page.url.pathname, SECTIONS, 'list'));
  const tabs = $derived(SECTIONS.map((sec) => ({ ...sec, href: resolve(sec.href) })));
</script>

<div class="flex flex-col gap-4 lg:gap-6">
  <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
    <div class="min-w-0 flex-1 flex flex-col gap-1">
      <AccountPageHeading>{s.title}</AccountPageHeading>
      <p class="text-sm text-muted-foreground max-lg:sr-only">
        {s.description}
      </p>
    </div>
    <Button
      variant="ghost"
      onclick={() => openCvIntake()}
      class="self-start shrink-0 rounded-lg bg-foreground px-3 text-background hover:bg-foreground hover:text-background hover:opacity-90"
    >
      {s.tailorForJob}
    </Button>
  </div>

  <TabStrip {tabs} {active} label={s.tabStripLabel} panelId={PANEL_ID} />

  <div role="tabpanel" id={PANEL_ID} aria-labelledby={tabStripId(PANEL_ID, active)} tabindex="0">
    {@render children()}
  </div>
</div>

{#if cvIntakeDialog.open}
  <JdIntakeDialog onClose={closeCvIntake} baseCvId={cvIntakeDialog.baseCvId} />
{/if}
