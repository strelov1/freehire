<script lang="ts">
  // Shown where push is chosen as a channel while the account has no registered
  // device. The backend refuses NEW push choices without one (409), but a choice
  // made earlier stays, and delivery skips it without a word — so this line is
  // the only place the user learns their push alerts reach nobody.
</script>

<p class="text-xs text-muted-foreground">
  No phone is set up for push, so push alerts reach nobody yet. Push will work after the next freehire app update: then open Profile in the app and turn on Push notifications.
</p>
