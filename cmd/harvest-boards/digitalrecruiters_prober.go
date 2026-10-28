package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// digitalrecruitersProber validates a DigitalRecruiters career site (the board is its host) the
// way internal/ingest/sources/digitalrecruiters.go reads it, which is unexported: the site's
// configuration on api.digitalrecruiters.com (404 when the platform does not know the host),
// then its listing, counting only the postings the site OWNS — an aggregating site lists other
// sites' postings, each naming its owner in career_domain, and the adapter leaves those to the
// owner's board. The employer is the brand of the site's first own posting.
//
// An offline site and an internal-mobility site (is_internal, for the employer's own staff) are
// declined: the adapter fails both.
type digitalrecruitersProber struct{}

const drProbeAPI = "https://api.digitalrecruiters.com"

var drProbeBoard = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// drProbeLocales mirrors the adapter's site-locale-to-API-locale map, language first.
var drProbeLocales = map[string]string{
	"cs": "cs_CZ", "da": "da_DK", "de": "de_DE", "en": "en_GB", "es": "es_ES", "it": "it_IT",
	"ja": "ja_JP", "ko": "ko_KR", "fr": "fr_FR", "nl": "nl_BE", "pl": "pl_PL", "pt": "pt_PT",
	"ro": "ro_RO", "sv": "sv_SE", "tr": "tr_TR",
}

func drProbeLocale(code string) string {
	code = strings.ReplaceAll(strings.TrimSpace(code), "-", "_")
	if strings.Contains(code, "_") {
		return code
	}
	if l, ok := drProbeLocales[code]; ok {
		return l
	}
	return code
}

func (digitalrecruitersProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	board = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(board), "/"))
	if !drProbeBoard.MatchString(board) {
		return "", 0, nil
	}
	var site struct {
		IsOnline      bool `json:"is_online"`
		IsInternal    bool `json:"is_internal"`
		DefaultLocale struct {
			ISOCode string `json:"iso_code"`
		} `json:"default_locale"`
	}
	if err := c.GetJSON(ctx, drProbeAPI+"/careers/v1/careers-sites/"+url.PathEscape(board), &site); err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	if !site.IsOnline || site.IsInternal || site.DefaultLocale.ISOCode == "" {
		return "", 0, nil
	}
	locale := drProbeLocale(site.DefaultLocale.ISOCode)
	var firstOwn int64
	owned, read := 0, 0
	for page := 1; page <= 50; page++ {
		q := url.Values{"domainName": {board}, "locale": {locale}, "limit": {"1000"}, "page": {strconv.Itoa(page)}}
		var listing struct {
			Count int `json:"count"`
			Items []struct {
				JobAdID      int64  `json:"job_ad_id"`
				CareerDomain string `json:"career_domain"`
			} `json:"items"`
		}
		if err := c.PostJSON(ctx, drProbeAPI+"/public/v1/careers-site/job-ads?"+q.Encode(), map[string]any{"filters": map[string]any{}}, &listing); err != nil {
			return "", 0, err
		}
		for _, it := range listing.Items {
			if strings.EqualFold(strings.TrimSpace(it.CareerDomain), board) {
				owned++
				if firstOwn == 0 {
					firstOwn = it.JobAdID
				}
			}
		}
		read += len(listing.Items)
		if read >= listing.Count || len(listing.Items) == 0 {
			break
		}
	}
	if owned == 0 {
		return "", 0, nil
	}
	var posting struct {
		BrandName string `json:"brand_name"`
	}
	q := url.Values{"domainName": {board}, "locale": {locale}}
	if err := c.GetJSON(ctx, drProbeAPI+"/public/v1/careers-site/job-ads/"+strconv.FormatInt(firstOwn, 10)+"?"+q.Encode(), &posting); err != nil {
		return "", 0, err
	}
	return strings.TrimSpace(posting.BrandName), owned, nil
}
