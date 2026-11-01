## 1. Exclusions

- [x] 1.1 Skip and clear postings with an empty company slug
- [x] 1.2 Skip and clear `via_recruiter` postings and postings of a company with two or more
- [x] 1.3 Report every skip reason separately

## 2. Matching

- [x] 2.1 `jobhash.TitleKeys`: punctuation-blind key, trailing clause optional
- [x] 2.2 Stemmed word overlap in the cross-check's board index
- [x] 2.3 Tests from the prod precision sample

## 3. Batch gates

- [x] 3.1 Judge a whole source per company (buffer the source)
- [x] 3.2 Corroboration, absent-share and minimum-board gates, measured on prod
- [x] 3.3 Clear every stamp a run does not re-stamp

## 4. Measurement

- [x] 4.1 Dry runs on prod before and after, precision sample of stamped postings
