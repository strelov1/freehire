<script lang="ts">
  import EmployerView from '$lib/components/EmployerView.svelte';
</script>

<!-- The account shell (my/+layout) owns the container, auth gate, and noindex;
     an inner max-width keeps the content readable within the content column. -->
<div class="max-w-3xl">
  <EmployerView />
</div>
