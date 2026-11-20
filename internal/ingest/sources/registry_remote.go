package sources

// remoteBoardSources are the remote-work boards and marketplaces: boardless, one global feed,
// company per posting (see All).
func remoteBoardSources(c HTTPClient) []Source {
	return []Source{
		NewRemoteOK(c),
		NewJobicy(c),
		NewWeWorkRemotely(c),
		NewNoDesk(c),
		NewJobspresso(c),
		NewJustRemote(c),
		// SkipTheDrive: WordPress remote board; listing = category pages, hydrating (body, location
		// and the employer's apply link are on the posting page). See skipthedrive.go.
		NewSkipTheDrive(pacedSkipTheDrive(c)),
		NewHubstaffTalent(c),
		NewFlexProfessionals(c),
		NewCrossover(c),
		NewWorkingNomads(c),
		NewHimalayas(c),
		NewRemotive(c),
		NewRemotedotcom(c),
		NewRemotli(c),
	}
}
