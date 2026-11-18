// Generator tools live in their own module so tygo and modernc.org/sqlite stay out of the
// main module's graph: `go build ./cmd/...` and the production image never fetch them.
// Run from this directory (paths are relative to it); see `make gen-contracts` / `make gen-metros`.
module github.com/strelov1/freehire/tools

go 1.27

toolchain go1.27.4

require (
	github.com/gzuidhof/tygo v0.2.21
	github.com/strelov1/freehire v0.0.0
	modernc.org/sqlite v1.61.1
)

require (
	github.com/JohannesKaufmann/dom v0.3.1 // indirect
	github.com/JohannesKaufmann/html-to-markdown/v2 v2.5.2 // indirect
	github.com/abadojack/whatlanggo v1.0.1 // indirect
	github.com/andybalholm/brotli v1.2.7 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/bdandy/go-errors v1.2.2 // indirect
	github.com/bdandy/go-socks4 v1.2.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bogdanfinn/fhttp v0.7.3 // indirect
	github.com/bogdanfinn/quic-go-utls v1.0.13-utls // indirect
	github.com/bogdanfinn/tls-client v1.18.0 // indirect
	github.com/bogdanfinn/utls v1.8.2-barnius // indirect
	github.com/bogdanfinn/websocket v1.5.9-barnius // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/dlclark/regexp2 v1.12.0 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/fatih/structtag v1.2.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gofiber/fiber/v2 v2.52.16 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.31.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.12.0 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/jaytaylor/html2text v0.0.0-20230321000545-74c2419ad056 // indirect
	github.com/klauspost/compress v1.20.3 // indirect
	github.com/klauspost/cpuid/v2 v2.4.0 // indirect
	github.com/klauspost/crc32 v1.3.0 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	github.com/microcosm-cc/bluemonday v1.0.27 // indirect
	github.com/minio/crc64nvme v1.1.1 // indirect
	github.com/minio/md5-simd v1.1.2 // indirect
	github.com/minio/minio-go/v7 v7.4.1 // indirect
	github.com/molecule-man/go-brrr v1.3.0 // indirect
	github.com/mozillazg/go-unidecode v0.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/olekukonko/tablewriter v0.0.5 // indirect
	github.com/pandatix/go-abnf v0.5.3 // indirect
	github.com/pgvector/pgvector-go v0.4.1 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pkoukk/tiktoken-go v0.1.6 // indirect
	github.com/prometheus/client_golang v1.25.0 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.73.1 // indirect
	github.com/prometheus/procfs v0.22.0 // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/richardlehane/mscfb v1.0.9 // indirect
	github.com/richardlehane/msoleps v1.0.6 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/ssor/bom v0.0.0-20170718123548-6386211fdfcf // indirect
	github.com/tam7t/hpkp v0.0.0-20160821193359-2b70b4024ed5 // indirect
	github.com/tiendc/go-deepcopy v1.7.2 // indirect
	github.com/tinylib/msgp v1.6.4 // indirect
	github.com/tmc/langchaingo v0.1.15 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.77.0 // indirect
	github.com/xuri/efp v0.0.3-0.20260924042702-217b81952dec // indirect
	github.com/xuri/excelize/v2 v2.11.1 // indirect
	github.com/xuri/nfp v0.0.2-0.20250530014748-2ddeb826f9a9 // indirect
	github.com/yuin/goldmark v1.8.7 // indirect
	github.com/zeebo/xxh3 v1.1.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.23.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/sdk v1.47.0 // indirect
	go.opentelemetry.io/otel/sdk/log v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.1 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/oauth2 v0.37.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/grpc v1.84.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/ini.v1 v1.67.3 // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

replace github.com/strelov1/freehire => ../
