<script lang="ts">
  import EmployerView from '#lib/components/EmployerView.svelte';
</script>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex.
     Full column width, like every other section, so card edges do not move between pages. -->
<div>
  <EmployerView section="vacancies" />
</div>
