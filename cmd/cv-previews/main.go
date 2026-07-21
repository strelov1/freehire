// Command cv-previews renders each registered CV template into a static SVG thumbnail from a
// fixed sample résumé, writing <out>/<id>.svg. The gallery in the tailoring workspace serves
// these committed SVGs, so re-run this (make cv-previews) whenever a template changes. Needs
// the typst binary on PATH (or TYPST_BIN); no database.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"

	"github.com/strelov1/freehire/internal/cv"
)

func main() {
	out := flag.String("out", "web/static/cv-previews", "directory to write <id>.svg previews into")
	flag.Parse()

	binName := os.Getenv("TYPST_BIN")
	if binName == "" {
		binName = "typst"
	}
	bin, err := exec.LookPath(binName)
	if err != nil {
		log.Fatalf("cv-previews: typst binary %q not found on PATH: %v", binName, err)
	}

	written, err := cv.GeneratePreviews(context.Background(), cv.NewTypstRenderer(bin), *out)
	if err != nil {
		log.Fatalf("cv-previews: generate: %v", err)
	}
	log.Printf("cv-previews: wrote %d previews to %s: %v", len(written), *out, written)
}
