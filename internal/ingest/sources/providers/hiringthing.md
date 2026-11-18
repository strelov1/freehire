**HiringThing traps** (`hiringthing`; verified 2026-11-11):

- **The native filter state proves the root listing is complete.**
  `HiringThing.Components.JobFiltersContainer` carries `showAll: true`, a `titles`
  array with every native ID, and the `remotePositions` / `nonRemotePositions`
  partition. The adapter requires all fields, valid distinct IDs, matching
  partitions, and exactly the same IDs in same-host posting links. SKIJapan's
  live root carried 16 titles and 16 posting links. No pagination is needed when
  that unfiltered state agrees; a filtered, missing or partial state fails the
  board rather than reporting a shortened list. This earns `fullBoardListing`.
- **Empty means explicit native emptiness.** Both
  `the-centennial-hotel.prismhr-hire.com` and
  `global-merchandising-services-inc.gnahiring.com` answered 200 with all three
  native arrays empty and a `careers-no-open-positions` element stating
  "No open positions at this time". Both signals are required before an empty
  result qualifies for `emptyBoardListing`; a bare page shell does not qualify.
- **Unreadable detail is not removal.** A transport failure, missing posting
  record, or description with no visible text yields an unreadable marker.
  Only the platform's own 404/410 drops a listed posting. This retains the
  pipeline's existing unreadable-share guard on board closure.
- **A quiet board can postpone a repair crawl.** Read `quiet_until` before a
  scoped run. Any deliberate repair exception must reset only the exact board's
  quiet pause, after rechecking its native state; do not clear provider-wide
  failures or alter the 48-hour unseen window.
