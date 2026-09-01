// Command mail-preview renders every outgoing email to HTML files you can open in
// a browser.
//
// Unlike the rest of cmd/, it is a development tool: it touches no database, needs
// no environment, and is never scheduled. Run it after changing a mail template:
//
//	go run ./cmd/mail-preview          # write the previews, then open index.html
//	go run ./cmd/mail-preview -dir X   # write them somewhere else
//
// The output directory is committed, because design-system/.storybook reads it and
// a test asserts the files still match what the templates render.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/strelov1/freehire/internal/engage/mailpreview"
)

func main() {
	dir := flag.String("dir", mailpreview.DefaultDir, "directory to write the previews into")
	base := flag.String("base", mailpreview.DefaultBaseURL,
		`origin the mails link to; "." makes the output self-contained for opening from disk`)
	flag.Parse()

	samples, err := mailpreview.Samples(*base)
	if err != nil {
		log.Fatalf("mail-preview: %v", err)
	}
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		log.Fatalf("mail-preview: %v", err)
	}
	if err := copyAssets(*dir); err != nil {
		log.Fatalf("mail-preview: %v", err)
	}

	// Three files per mail: the one that gets sent (which follows the reader's
	// preference) plus a pinned copy per scheme, which is what the contact sheet's
	// light/dark toggle switches between. Swapping a file is the only way to force a
	// scheme inside a frame — a page cannot override what the OS reports, and reading
	// a sibling file to rewrite it is blocked when the sheet is opened off disk.
	for _, s := range samples {
		for suffix, body := range map[string]string{
			".html":       s.HTML,
			".light.html": s.LightHTML,
			".dark.html":  s.DarkHTML,
		} {
			path := filepath.Join(*dir, s.Name+suffix)
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				log.Fatalf("mail-preview: writing %s: %v", path, err)
			}
		}
	}

	index := filepath.Join(*dir, "index.html")
	if err := os.WriteFile(index, []byte(mailpreview.Index(samples)), 0o644); err != nil {
		log.Fatalf("mail-preview: writing %s: %v", index, err)
	}

	fmt.Printf("wrote %d previews to %s\nopen %s\n", len(samples), *dir, index)
}

// copyAssets places every image the mails reference beside the previews, so a
// relative -base resolves them and the Storybook copies do not depend on the
// production host being reachable.
func copyAssets(dir string) error {
	for _, name := range []string{
		"email-logo.png",
		"email-icon-github.png",
		"email-icon-discord.png",
		"email-icon-linkedin.png",
		"ilya.jpg",
	} {
		b, err := os.ReadFile(filepath.Join("web/static", name))
		if err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
