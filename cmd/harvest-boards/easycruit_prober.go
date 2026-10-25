package main

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// easycruitProber validates a Visma EasyCruit career site (the <board> of lentis.easycruit.com)
// by reading the list export the adapter reads, /export/xml/vacancy/list.xml, and counting its
// vacancies. It mirrors internal/ingest/sources/easycruit.go, which is unexported.
//
// The export names no site title; the employer is read from the first vacancy's department (the
// customer's own unit name), so a site with no openings is reported without one. An unknown
// label answers 404: a skip.
type easycruitProber struct{}

var easycruitBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func (easycruitProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !easycruitBoardPattern.MatchString(board) {
		return "", 0, nil
	}
	var list struct {
		XMLName   xml.Name `xml:"VacancyList"`
		Vacancies []struct {
			Company     string   `xml:"Versions>Version>AlternativeCompanyName"`
			Departments []string `xml:"Departments>Department>Name"`
		} `xml:"Vacancy"`
	}
	if err := c.GetXML(ctx, "https://"+board+".easycruit.com/export/xml/vacancy/list.xml", &list); err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	name := ""
	if len(list.Vacancies) > 0 {
		v := list.Vacancies[0]
		name = strings.TrimSpace(v.Company)
		if name == "" && len(v.Departments) > 0 {
			name = strings.TrimSpace(v.Departments[0])
		}
	}
	return name, len(list.Vacancies), nil
}

// discover enumerates candidate sites from Common Crawl's index of *.easycruit.com.
func (easycruitProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "*.easycruit.com", recognizedBoard("easycruit"))
}
