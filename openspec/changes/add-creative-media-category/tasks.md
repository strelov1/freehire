## 1. Vocabulary

- [x] 1.1 Add `creative` to `vocab.CategoryValues` and to `vocab.TechCategories`, with a comment saying what it covers and why it is technical; confirm `vocab_test.go`'s partition test passes unchanged.

## 2. Title classification

- [x] 2.1 Add the video family aliases (`video editor`, `video producer`, `videographer`) to `classify`'s `categoryTable` → `creative`, with tests.
- [x] 2.2 Add the art family aliases (`3d artist`, `2d artist`, `concept artist`, `character artist`, `environment artist`, `technical artist`, `vfx artist`, `storyboard artist`) → `creative`, with tests.
- [x] 2.3 Add the animation aliases (`animator`, `3d animator`, `2d animator`, `motion graphics artist`) → `creative`, with tests.
- [x] 2.4 Move the audio spellings from `design` to `creative` — `sound designer`, `audio designer`, and the two "…Design Engineer" forms that otherwise fall into draughting — with tests. Bare `audio engineer` / `sound engineer` are deliberately NOT aliases: broadcast and AV integration use them.
- [x] 2.5 Add `photographer`, `photo editor` and `illustrator` → `creative`, with the collision test for "Graphic Designer (Illustrator, Photoshop)" staying `design`.
- [x] 2.6 Add regression tests that the bare craft words (`audio`, `video`, `art`, `sound`) resolve nothing, and that `motion designer`, `graphic designer`, `visual designer`, `brand designer`, `content creator` and `ugc creator` keep their current category.

## 3. Named roles

- [x] 3.1 Add the media-craft roles to `roletag` (`video_editor`, `videographer`, `video_producer`, `animator`, `3d_artist`, `concept_artist`, `technical_artist`, `storyboard_artist`, `illustrator`, `photographer`, `sound_designer`), ordered so no entry steals a longer alias, with tests.
- [x] 3.2 Add the game-development roles (`game_designer`, `level_designer`, `narrative_designer`, `game_producer`, `game_developer`) with tests asserting each keeps the category it resolves to today.

## 4. Skill dictionary

- [x] 4.1 Add the creative canonicals to `skilltag` (`davinci-resolve`, `final-cut-pro`, `cinema-4d`, `capcut`, `godot`, `houdini`, `substance-painter`, `substance-designer`, `zbrush`, `color-grading`, `storyboarding`, `video-editing`), with tests. `nuke` is deliberately omitted.
- [x] 4.2 Place each entry in the tier that can actually hold it: `houdini` and `c4d` in `ambiguousWords` (word pass), the three craft names in `nonCorroboratingPhrases` (phrase pass), `animation` and `nuke` omitted entirely — with a test for each, including the false context it collides with.

## 5. Contracts and web

- [x] 5.1 Regenerate `web/src/lib/generated/contracts.ts` via `cmd/gen-contracts` and verify `creative` and the new role labels are present.
- [x] 5.2 Add the `creative` label to `web/src/lib/labels.ts` and place it in `web/src/lib/filterSections.ts`'s craft group (renamed "Design & Creative"); run `svelte-check` to prove the category is selectable.
- [x] 5.3 Add the same label to `extension/lib/labels.ts`, whose `CATEGORY_LABELS` is exhaustive by contract so a code never renders two ways across the two apps, and to the category vocabulary listed in `docs/API.md`.

## 6. Verification

- [x] 6.1 Run `gofmt -l .`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...` and `golangci-lint run`; all clean.
- [x] 6.2 Write the rollout note into the change (backfill-derive → stop reindex timer → plain `make reindex` → start timer) and record the pre-deploy estimate to compare the post-backfill facet against.
