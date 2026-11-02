<script lang="ts">
  // A sample conversation with the connector. Illustrative, so no real employer is named.
  const jobs = [
    { title: 'Senior Go Engineer — payments platform', meta: 'Remote · EU · €85–100k' },
    { title: 'Senior Backend Engineer (Go, Kafka)', meta: 'Remote · Germany, Netherlands' },
    { title: 'Staff Go Developer — observability', meta: 'Remote · Europe · posted 2 days ago' },
  ];
</script>

<figure class="overflow-hidden rounded-xl border border-border bg-secondary/60 text-sm shadow-sm">
  <figcaption class="flex items-center gap-2 border-b border-border px-4 py-2.5 text-xs text-muted-foreground">
    <span class="size-2.5 rounded-full bg-muted-foreground/30"></span>
    Your assistant · freehire connector
  </figcaption>
  <div class="flex flex-col gap-3 p-4 leading-relaxed">
    <div class="max-w-xs self-end rounded-2xl rounded-br-sm bg-foreground px-3.5 py-2 text-background">
      Remote senior Go jobs in Europe, posted this week
    </div>
    <p class="text-xs text-muted-foreground">Used freehire · Search jobs</p>
    <div class="max-w-md rounded-2xl rounded-bl-sm border border-border bg-background px-3.5 py-2.5">
      <p class="text-muted-foreground">Here are the strongest matches:</p>
      {#each jobs as job (job.title)}
        <div class="mt-2.5">
          <p class="font-medium text-foreground">{job.title}</p>
          <p class="text-xs text-muted-foreground">{job.meta}</p>
        </div>
      {/each}
    </div>
    <div class="max-w-xs self-end rounded-2xl rounded-br-sm bg-foreground px-3.5 py-2 text-background">
      Save the first one to my board
    </div>
    <p class="self-end text-xs text-muted-foreground">Saving needs the account connector</p>
  </div>
</figure>
