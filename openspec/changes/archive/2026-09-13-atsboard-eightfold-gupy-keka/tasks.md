## 1. Recognition

- [x] 1.1 Add `keka.com` → `keka` as a `subdomain`-mode entry in `atsBoards`
- [x] 1.2 Add a `TestRecognize` case (vacancy/careers-path URL → tenant board, canonical
      collapses to the bare host)
- [x] 1.3 ~~Add `eightfold.ai` → `eightfold` and `gupy.io` → `gupy`~~ — reverted after review
      found both adapters key on something other than the bare subdomain label (see
      proposal.md's revision note); not part of this change

## 2. Verification

- [x] 2.1 Confirm `internal/ingest/atsdetect`'s `TestLocalShapesStayOutsideTheSharedTable`
      still passes (guards that Paycom, Oracle, Taleo, NEOGOV, and Comeet stay outside the
      shared table)
- [x] 2.2 Add a regression test for Keka's known non-tenant host (`app.keka.com`, already
      declined by the existing generic `"app"` platform label)
- [x] 2.3 Run the full consumer set: `atsboard`, `boardresolve`, `linksource`, `contribution`,
      `atsdetect`
