<script lang="ts">
  // `control` is where the row's real behavior lives — a story that faked it with
  // raw markup would show none of what callers actually put there.
  import SettingRow from '../../src/setting-row.svelte';
  import Input from '../../src/input.svelte';

  let {
    label = 'Font',
    hint,
    grow = false,
  }: { label?: string; hint?: string; grow?: boolean } = $props();
</script>

<div class="max-w-sm">
  <SettingRow {label} {hint} {grow}>
    {#snippet control()}
      <Input value="Inter" class={grow ? 'w-full' : 'w-20'} />
    {/snippet}
  </SettingRow>
</div>
