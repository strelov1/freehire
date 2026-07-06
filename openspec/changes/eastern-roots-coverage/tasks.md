# Tasks

## 1. Source boards

- [x] 1.1 Add live-validated boards: Chatfuel (`breezy.yml`), Kommo + Emerging
  Travel Group (`workable.yml`), Jooble + Let's Enhance + Headway Inc
  (`teamtailor.yml`).
- [x] 1.2 Disambiguate generic-slug merges: rename `lever` `ajax` → Ajax Systems,
  `personio` `vivid` → Vivid Money, `breezy` `gen-tech` → Genesis Tech.

## 2. Collection membership

- [x] 2.1 Add 10 eastern-roots slugs to `eastern_roots.txt`, alphabetically
  placed, no duplicates.

## 3. Verify

- [x] 3.1 All five board files parse as YAML; new/renamed entries are unique.
- [x] 3.2 `go build ./...` and `go test ./internal/collections/` green.
