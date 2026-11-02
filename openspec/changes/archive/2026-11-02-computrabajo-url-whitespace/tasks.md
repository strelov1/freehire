# Tasks

- [x] Reproduce the baseline production error with a failing regression test.
- [x] Normalize the posting href and pass the regression, Go unit tests, both vet checks and lint.
- [x] Apply simplify and review the complete diff.
- [x] Pass the full integration-tagged suite through the existing same-host SSH Docker access.
- [x] Repeat duplicate checks and prepare the validated change and PR description for publication.

All 270 tested packages passed in the full integration run. Docker was reached through a
private temporary Unix socket forwarded over existing SSH access; production permissions
and configuration were unchanged. PR publication is tracked in the thread's finish-task state.
