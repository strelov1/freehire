## REMOVED Requirements

### Requirement: Role facet on the jobs index and search endpoint

**Reason**: Used on 1.6% of searches — 894 of 54,870 over two days of production logs,
against 8,782 for `category` on the same sample. A year of dictionary work did not move
that number: the figure recorded when the facet's suggestions were built was 1.1%.

**Migration**: A request still carrying `role=`, `role_exclude=` or `role_mode=` is not
refused. It lands in `meta.ignored_params`, the mechanism that already reports a dropped
filter, so a stale saved search or a shared link says what happened rather than silently
returning the whole catalogue. `category` and `seniority` answer the same question on
the same request.
