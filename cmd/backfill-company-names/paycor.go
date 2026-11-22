package main

import (
	"context"
	"log"
	"regexp"

	"github.com/strelov1/freehire/internal/dict/companyname"
)

const paycorVendor = "Paycor"

var paycorClientID = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)

// paycorResolver names a Paycor board from its listing title when the tenant branded it, and
// from the posting text otherwise. A board id that is not a 32-hex clientId is refused before
// either step: the title step puts it into a URL built from nothing else.
type paycorResolver struct {
	title, postings companyname.Resolver
}

// newPaycorResolver skips the title step when title is nil.
func newPaycorResolver(title, postings companyname.Resolver) companyname.Resolver {
	return paycorResolver{title: title, postings: postings}
}

func (r paycorResolver) Name(ctx context.Context, board string) (string, error) {
	if !paycorClientID.MatchString(board) {
		return "", nil
	}
	if r.title != nil {
		name, err := r.title.Name(ctx, board)
		if err != nil {
			// A listing that is down says nothing about the postings already stored.
			log.Printf("resolve paycor %s title: %v", board, err)
		}
		// A title the later gate would refuse, or one that is the vendor's, falls through.
		if name, ok := companyname.AcceptPlaceholder(board, name); ok && usableEmployerName(name, paycorVendor) {
			return name, nil
		}
	}
	return r.postings.Name(ctx, board)
}
