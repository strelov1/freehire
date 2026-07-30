<script lang="ts">
  // Table renders the table/thead/tbody and leaves rows and cells to the caller,
  // handing out `tr`/`th`/`td` as classes. Only a real caller shows that shape.
  import Table, { tableVariants } from '../../src/table.svelte';
  import Badge from '../../src/badge.svelte';

  type Row = { role: string; company: string; status: string };

  let {
    rows = [
      { role: 'Senior Go Engineer', company: 'Granola', status: 'Applied' },
      { role: 'Platform Engineer', company: 'Meilisearch', status: 'Screening' },
      { role: 'Backend Engineer', company: 'Fly.io', status: 'Rejected' },
    ],
  }: { rows?: Row[] } = $props();

  const slots = tableVariants();
</script>

<Table>
  {#snippet header()}
    <tr class={slots.tr()}>
      <th class={slots.th()}>Role</th>
      <th class={slots.th()}>Company</th>
      <th class={slots.th()}>Status</th>
    </tr>
  {/snippet}
  {#each rows as row (row.role)}
    <tr class={slots.tr()}>
      <td class={slots.td()}>{row.role}</td>
      <td class={slots.td()}>{row.company}</td>
      <td class={slots.td()}><Badge variant="secondary">{row.status}</Badge></td>
    </tr>
  {/each}
</Table>
