// Package mcpapp is the MCP server freehire's ChatGPT app is built on.
//
// It is the SECOND MCP transport in this repository, and the distinction matters.
// internal/api/ojcpmcp serves the Open Job Context Protocol: a foreign schema, a
// conformance suite, and an audience of agents that branch on error codes. This one serves
// OpenAI's Apps SDK, whose audience is a language model reading prose, and it answers in
// OUR wire shapes — the ones a visitor to the site would see — because OJCP's cannot hold
// them: 34 of jobview.Job's fields have no home in that standard, and 18.1% of open
// postings carry a seniority its vocabulary cannot name.
//
// Like ojcpmcp it is an ADAPTER and nothing else. Every tool loads through a Reader the
// REST handlers satisfy, so the two servers cannot disagree about what is in the catalogue
// — only about how they publish it, which is the whole reason there are two.
package mcpapp
