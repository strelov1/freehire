## 1. Ashby: workplaceType decides the work mode

- [x] 1.1 Failing test: a posting with `workplaceType=Hybrid` and `isRemote=true` yields
      `work_mode=hybrid` and is not remote (`ashby_test.go`)
- [x] 1.2 Decode `workplaceType` on `AshbyPosting`; resolve the mode via
      `firstNonEmpty(workplaceTypeMode, workModeFromRemote)` and derive `Remote` from it
- [x] 1.3 Confirm `internal/linksource` inherits the fix through the shared
      `MapAshbyPosting`

## 2. Recruitee and SmartRecruiters: read the hybrid flag

- [x] 2.1 Failing test: a Recruitee offer with `remote=false, hybrid=true` yields
      `work_mode=hybrid` (`recruitee_test.go`)
- [x] 2.2 Add the `workModeFromRemoteHybrid` helper; both-false yields `""`
- [x] 2.3 Failing test: a SmartRecruiters posting with `location.hybrid=true` yields
      `work_mode=hybrid` (`smartrecruiters_test.go`)
- [x] 2.4 Decode `hybrid` on both adapters and route them through the helper
- [x] 2.5 Unit-test the helper's four input combinations (`workmode_test.go`)

## 3. BambooHR: read locationType

- [x] 3.1 Failing test: careers-list postings with `locationType` `0`/`1`/`2` and null
      `isRemote` yield `onsite`/`remote`/`hybrid` (`bamboohr_test.go`)
- [x] 3.2 Decode `locationType`, add the `bambooHRLocationType` mapper, keep `isRemote`
      as the fallback

## 4. Quality pass and verification

- [x] 4.1 Run the `simplify` skill over the diff; tests stay green
- [x] 4.2 `gofmt -l`, `go vet ./...`, `go build ./...`, `go test ./...` all clean
- [x] 4.3 Verify the four adapters against their live APIs — the Surfshark posting that
      triggered the report resolves to `hybrid`
- [x] 4.4 Code review on the full diff; fix Critical and Important findings

## 5. Finish

- [x] 5.1 Commit and open the PR (#1243); merge pending review
- [x] 5.2 Archive and sync the OpenSpec change
- [x] 5.3 Post-merge ops: deployed (release.sh, blue), re-ingested all four providers.
      Full reindex refused (free 32GiB < the 70GiB floor); incremental indexing at ingest
      carried the change into search, verified on the reported job
- [ ] 5.4 Offer a changelog entry (user-visible: hybrid jobs leave the remote filter)
