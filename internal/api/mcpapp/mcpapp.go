// Package mcpapp is the MCP server freehire's ChatGPT app is built on.
//
// It is the SECOND MCP transport. internal/api/ojcpmcp serves the Open Job Context Protocol
// (a foreign schema, agents that branch on error codes). This one serves OpenAI's Apps SDK,
// whose audience is a language model reading prose, and answers in OUR wire shapes because
// OJCP's cannot hold them (many jobview.Job fields and seniority values have no home there).
//
// Like ojcpmcp it is an ADAPTER and nothing else: every tool loads through a Reader the REST
// handlers satisfy, so the two servers differ only in how they publish the catalogue.
package mcpapp
