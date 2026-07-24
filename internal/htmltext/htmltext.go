// Package htmltext converts stored job-description HTML into alternative
// representations for API consumers: plain text and Markdown. Each converter is
// lenient — a conversion error degrades to returning the input unchanged rather
// than failing the caller.
package htmltext

import (
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/jaytaylor/html2text"
)

// ToText renders HTML as plain text with tags removed. On a conversion error it
// returns the input unchanged.
func ToText(html string) string {
	out, err := html2text.FromString(html, html2text.Options{OmitLinks: true})
	if err != nil {
		return html
	}
	return out
}

// ToMarkdown converts HTML to Markdown, preserving block structure such as lists
// and headings. On a conversion error it returns the input unchanged.
func ToMarkdown(html string) string {
	out, err := htmltomarkdown.ConvertString(html)
	if err != nil {
		return html
	}
	return out
}
