## 1. Sitemap decode

- [x] 1.1 Decode a sitemap body that is XML or gzip magic, reject any root other than `urlset` or `sitemapindex`, and cap uncompressed size

## 2. Adapter

- [x] 2.1 Crawl a sitemap URL, walk an index, resolve relative locs, and filter with the region pattern
- [x] 2.2 Hydrate new JobPosting pages up to the cap, emit seen URLs without fetching, and map fields including the board-company fallback
- [x] 2.3 Fail the board on a bad sitemap or a failed child, and refuse a non-URL board

## 3. Registration

- [x] 3.1 Register `sitemapldjson` as a hydrating board source, label it `Sitemap JSON-LD`, and regenerate source values
