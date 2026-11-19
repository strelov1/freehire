package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// kuper adapts Kuper's public vacancy API (vacancies-api.sbermarket.ru), a single-company source
// with no per-tenant board id (boardless). The list endpoint paginates by page and carries the
// description inline. Each page's result is a list of category blocks: the pagination block holds
// the page count, the vacancies block the postings.
type kuper struct {
	http JSONGetter
}

const (
	kuperListURL    = "https://vacancies-api.sbermarket.ru/api/vacancy_pagination/?page=%d"
	kuperVacancyURL = "https://kuper.ru/rabota/%s-%d"
)

// NewKuper builds the Kuper adapter over the given HTTP client.
func NewKuper(c JSONGetter) Source { return kuper{http: c} }

func (kuper) Provider() string { return "kuper" }

// kuper is single-company, so its config entries carry no board.
func (kuper) Boardless() {}

// kuperVacancy is one posting from the vacancies block (description inline).
type kuperVacancyItem struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	City        string   `json:"city"`
	WF          []string `json:"wf"`
	FriendlyURL string   `json:"friendlyUrl"`
	IDForURL    int64    `json:"idForUrl"`
}

func (k kuper) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; ; page++ {
		var resp struct {
			Result []struct {
				Category string          `json:"category"`
				Data     json.RawMessage `json:"data"`
			} `json:"result"`
		}
		if err := k.http.GetJSON(ctx, fmt.Sprintf(kuperListURL, page), &resp); err != nil {
			return nil, fmt.Errorf("kuper: list page %d: %w", page, err)
		}

		pages := 1
		var vacancies []kuperVacancyItem
		for _, blk := range resp.Result {
			switch blk.Category {
			case "pagination":
				var p struct {
					Pages int `json:"pages"`
				}
				if err := json.Unmarshal(blk.Data, &p); err != nil {
					return nil, fmt.Errorf("kuper: page %d pagination block: %w", page, err)
				}
				pages = p.Pages
			case "vacancies":
				if err := json.Unmarshal(blk.Data, &vacancies); err != nil {
					return nil, fmt.Errorf("kuper: page %d vacancies block: %w", page, err)
				}
			}
		}

		for _, v := range vacancies {
			jobs = append(jobs, Job{
				ExternalID: v.ID,
				// Best-effort public URL: kuper.ru 403s bots, so this shape is UNCONFIRMED
				// against the live site.
				URL:         fmt.Sprintf(kuperVacancyURL, v.FriendlyURL, v.IDForURL),
				Title:       v.Title,
				Company:     e.Company,
				Location:    v.City,
				Description: sanitizeHTML(v.Description),
				Remote:      isRemote(strings.Join(v.WF, " ")),
				PostedAt:    nil, // source carries no date
			})
		}

		if page >= pages {
			break
		}
	}
	return jobs, nil
}
