## 1. Crawl all top-level categories

- [x] 1.1 Add a `tbankCategories` curated constant slice (`tcareer_it`,
  `tcareer_back_office`, `tcareer_work_with_clients`) and set the `getVacancies`
  list request's `filters.category` to it; correct the false "publisher covers all
  roles" comment to document the category-filter requirement.
- [x] 1.2 Cover the behavior in `tbank_test.go`: assert the list request body sends
  all three categories in `filters.category` (RED-first), and that pagination still
  drains vacancies across the filtered set into mapped Jobs.
