**Recruitee traps** (`recruitee`):

- **The only adapter that yields `Job.ApplyForm`.** Its `/api/offers/` response holds `open_questions` and the `options_*` standard-field flags beside the posting, so the form costs no extra request and `applyform.FromRecruitee` maps it in place.
- **Empty board** (`emptyBoardListing`): `{"offers":[]}`; an unknown board 404s.
