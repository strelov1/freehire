package sources

import (
	"context"
	"net/url"

	"golang.org/x/net/html"
)

// Only the destinations verified in the PageUp migration ledger use the proxy lane.
var clinchMigratedHosts = map[string]bool{
	"careers.madergroup.com.au":                                        true,
	"armaguard-careers.careerpages.rec-marketing.dc2.pageuppeople.com": true,
	"jobs.citadel.edu":                                                 true,
	"careers.dha.gov.au":                                               true,
	"careers.savills.com.au":                                           true,
	"jobs.du.edu":                                                      true,
	"psacareers.singaporepsa.com":                                      true,
	"careers.wintv.com.au":                                             true,
	"jobs.boisestate.edu":                                              true,
	"careers.duratec.com.au":                                           true,
	"careers.marist.edu":                                               true,
	"careers.mater.org.au":                                             true,
	"careers.ahpra.gov.au":                                             true,
	"careers.moretonbay.qld.gov.au":                                    true,
	"careers.northline.com.au":                                         true,
	"external-jobs.csu.edu.au":                                         true,
	"north-idaho-college-careers.career-pages.com":                     true,
	"careers.drexel.edu":                                               true,
	"careers.artscentremelbourne.com.au":                               true,
	"careers.whiddon.com.au":                                           true,
	"careers.knox.vic.gov.au":                                          true,
	"careers.linfox.com":                                               true,
	"jobs.sandiego.edu":                                                true,
	"jobs.yooralla.com.au":                                             true,
	"career.leightonasia.com":                                          true,
	"careers.worldvision.com.au":                                       true,
	"careers.utas.edu.au":                                              true,
	"jobs.amanaliving.com.au":                                          true,
	"schindler-lifts-australia-careers.careerpages.rec-marketing.dc2.pageuppeople.com": true,
	"careers.device.com.au":   true,
	"jobs.unicef.org":         true,
	"careers.kenyon.edu":      true,
	"careers.lwb.org.au":      true,
	"careers.fire.nsw.gov.au": true,
	"careers.tafensw.edu.au":  true,
	"careers.ses.nsw.gov.au":  true,
	"careers.rfs.nsw.gov.au":  true,
	"careers.icac.nsw.gov.au": true,
}

type clinchDetailHTTP struct {
	plain, migrated HTMLGetter
}

func (c clinchDetailHTTP) GetHTML(ctx context.Context, raw string) (*html.Node, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if clinchMigratedHosts[u.Hostname()] {
		return c.migrated.GetHTML(ctx, raw)
	}
	return c.plain.GetHTML(ctx, raw)
}
