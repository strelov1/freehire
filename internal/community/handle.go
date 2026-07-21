package community

import (
	"fmt"
	"math/rand/v2"
)

// handleAdjectives and handleNouns are the word banks for pseudonymous handles.
// Their sizes set the base collision space (|adj| × |noun| × 90); a DB-level
// UNIQUE(handle) plus a retry in the service turns "usually unique" into a
// guarantee, so the banks only need to be varied and neutral, not exhaustive.
var handleAdjectives = []string{
	"quiet", "brave", "calm", "clever", "eager", "gentle", "swift", "keen",
	"lucky", "mellow", "noble", "plucky", "spry", "witty", "zesty", "bold",
	"curious", "humble", "nimble", "sunny",
}

var handleNouns = []string{
	"otter", "falcon", "maple", "harbor", "ember", "willow", "pixel", "comet",
	"lynx", "cedar", "raven", "quartz", "meadow", "badger", "puffin", "thistle",
	"walrus", "cypress", "marmot", "beacon",
}

// GenerateHandle returns a fresh pseudonymous handle like "quiet-otter-42".
// math/rand/v2 needs no seeding and is safe for concurrent use.
func GenerateHandle() string {
	adj := handleAdjectives[rand.IntN(len(handleAdjectives))]
	noun := handleNouns[rand.IntN(len(handleNouns))]
	return fmt.Sprintf("%s-%s-%d", adj, noun, rand.IntN(90)+10)
}
