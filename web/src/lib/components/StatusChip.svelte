<!--
  The classified status of one email, as a chip. It is the ONE rendering of that datum:
  the inbox list, the landing preview, the suggestion card and the drawer's mail thread
  all reach it here rather than each spelling out `rounded border px-1.5` plus a call to
  statusClass.

  It also owns the rule that an unclassified signal — and `other`, and a signal from a
  server ahead of this build — renders NOTHING. That rule lived in a `{#if statusLabel(…)}`
  at every call site, which is three chances to forget it and show an empty bordered box.

  `class` is passed through for the type size, because the callers genuinely differ: the
  dense list rows set it themselves, while the drawer inherits it from a wrapper so the
  "→ Interview" explanation beside the chip sits on the same baseline. Colour comes last so
  the signal's own border/text win over the outline variant's defaults.
-->
<script lang="ts">
  import { Badge } from '$lib/ui';
  import { statusClass, statusLabel } from '$lib/emailStatus';

  let { signal, class: className = '' }: { signal?: string; class?: string } = $props();

  const label = $derived(statusLabel(signal));
</script>

{#if label}
  <Badge variant="outline" class="{className} {statusClass(signal)}">{label}</Badge>
{/if}
