## 1. Résumé read composition

- [x] 1.1 Add a store/read path that, when the structure stamp is stale, still yields contact fields from the superseded blob plus a pending flag (keep `Structured`’s `ok` semantics for seed gates)
- [x] 1.2 Update `GetResume` to compose bank experience + provisional contacts + pending/stale signal on the cookie résumé status payload; leave semantic sections empty while pending
- [x] 1.3 Unit tests: stale+bank+superseded contacts; stale+bank+no contacts; current structure unchanged; profile `cv` / Professional still contact-free

## 2. Profile UI

- [x] 2.1 Thread the pending signal through `getResume` types/client
- [x] 2.2 Profile tab: show pending/stale explanation; still render provisional contacts and banked experience when present

## 3. Extract window hygiene

- [x] 3.1 Reproduce why the latest local upload did not stamp `resume_structured` (logs / LLM / PII path) and fix or surface failure so the pending window ends
- [x] 3.2 Manual check on `/my/profile` Profile tab: name, phone, links visible during pending and after stamp catches up

## 4. Verification

- [x] 4.1 `go test` / `go vet -tags=integration` for affected packages; focused web check if types changed
