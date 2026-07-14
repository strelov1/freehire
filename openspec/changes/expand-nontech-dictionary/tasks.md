## 1. Expand the non-tech dictionary

- [x] 1.1 Grow `classify.nonTechTitleTerms` (internal/classify/nontech.go) across the eight measured clusters (healthcare, food, retail, warehouse/logistics, trades, office/finance, education, facilities), unambiguous role nouns only
- [x] 1.2 Extend the `IsNonTech` test: positives per cluster (Registered Nurse, Line Cook, Warehouse Associate, Order Picker, Ironworker, Paralegal, Bookkeeper, Substitute Teacher…) and trap negatives (IT Technician, Field Service Technician, Data Warehouse Engineer, Security Engineer, Systems Coordinator, Data Analyst → NOT non-tech)

## 2. Verify + ship

- [x] 2.1 `go build ./... && go vet ./... && go test ./...` green
- [ ] 2.2 Deploy, chained `backfill-derive && reindex`, measure new true/false/null split on prod
