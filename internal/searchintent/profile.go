package searchintent

// Profile is what the caller has already told us, in the form this package reads it.
//
// It is a plain struct rather than the stored profile type so the package keeps no
// dependency on the account layer: the handler maps one to the other, and the mapping
// is the only place that has to know both.
//
// Every field here is ALREADY canonical — the profile validates specializations against
// the category vocabulary and normalises skills on the way in — which is why FromProfile
// needs no model. See its doc comment.
type Profile struct {
	// Specializations are category values.
	Specializations []string
	// Skills are canonical skill tags.
	Skills []string
	// ExcludedSkills are the technologies the caller said they do not want to work
	// with. They become exclusions: importing them as wants would build precisely the
	// search they told us to avoid.
	ExcludedSkills []string
	// WorkModes are the arrangements they accept.
	WorkModes []string

	// The three geographies a profile holds answer different questions, and they are
	// carried apart because collapsing them changes what the profile says.
	//
	// RemoteFrom is the reach they will work remotely for and RelocateTo is where they
	// would move — both are places they asked for. BasedIn is where they live now,
	// which is NOT a search: someone in Lisbon who is open to relocating did not ask
	// for jobs in Portugal. It is carried so the distinction is visible in the type
	// rather than lost at the mapping, and deliberately not filtered on.
	RemoteFrom []string
	BasedIn    string
	Relocating bool
	RelocateTo []string
}
