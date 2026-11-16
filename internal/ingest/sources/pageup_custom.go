package sources

import (
	"context"
	"net/url"
)

type pageupCustomHTTP struct {
	plain, custom pageupHTTP
}

func (p pageupCustomHTTP) GetJSONWithHeaders(ctx context.Context, raw string, headers map[string]string, v any) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Host == "jobs.northmemorial.com" {
		return p.custom.GetJSONWithHeaders(ctx, raw, headers, v)
	}
	return p.plain.GetJSONWithHeaders(ctx, raw, headers, v)
}
