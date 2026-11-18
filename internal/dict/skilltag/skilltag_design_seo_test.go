package skilltag

import (
	"slices"
	"testing"
)

// The design craft's toolchain and the CAD/EDA stack the engineering-design side
// states. Both were nearly absent: the dictionary knew figma, photoshop and autocad
// and little else, so a designer's description came back with one tag or none.
//
// Three aliases stay OUT deliberately — "principle" (ordinary English), "eagle"
// (the bird, and "eagle-eyed" is posting boilerplate) and "nx" (Siemens NX vs. the
// Nx JS monorepo tool). Two go in behind the corroboration gate instead of being
// dropped: "sketch" ("sketch out ideas") and "maya" (a person's name).
func TestParse_DesignAndCADVocab(t *testing.T) {
	contains := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}
	cases := []struct {
		name   string
		in     string
		want   []string
		absent []string
	}{
		// design tools
		{"adobe suite", "You will work in Figma and Adobe Illustrator, with InDesign for print.",
			[]string{"figma", "illustrator", "indesign"}, nil},
		{"bare illustrator", "Strong Illustrator and Photoshop skills.", []string{"illustrator", "photoshop"}, nil},
		{"after effects", "Motion work in Adobe After Effects and Premiere Pro.", []string{"after-effects", "premiere-pro"}, nil},
		// "after effects" unqualified is clinical prose, and healthcare is the largest
		// non-technical mass this catalogue filters. A false STRONG token would also
		// have lifted the gate off the weak words beside it.
		{"after effects of anaesthesia",
			"Registered Nurse. Monitor patients for the after effects of anaesthesia; sketch out care plans.",
			nil, []string{"after-effects", "sketch"}},
		{"a solid edge over the competition",
			"Account Executive. Our platform gives clients a solid edge over the competition. Sketch out the territory plan.",
			nil, []string{"solid-edge", "sketch"}},
		{"invision as a misspelling of envision",
			"We invision a workplace where everyone belongs. Sketch out your growth plan.",
			nil, []string{"invision", "sketch"}},
		{"menu prototyping in a kitchen",
			"Line Cook. Menu prototyping with the chef; sketch out plating ideas.",
			nil, []string{"prototyping", "sketch"}},
		{"adobe xd", "Wireframes in Adobe XD.", []string{"adobe-xd", "wireframing"}, nil},
		{"no-code design", "Ship marketing pages in Webflow.", []string{"webflow"}, nil},
		{"handoff tools", "Design handoff through InVision and Zeplin, source in Figma.",
			[]string{"invision", "zeplin", "figma"}, nil},
		{"3d design", "3D assets in Blender, animations with Lottie built in Figma.",
			[]string{"blender", "lottie", "figma"}, nil},
		// design practices
		{"practices", "You will own prototyping and wireframing in Figma.",
			[]string{"prototyping", "wireframing", "figma"}, nil},
		// "design system(s)" is NOT a canonical: it is also the ordinary verb phrase
		// every backend and embedded posting writes ("you will design systems that
		// scale"). A phrase match is always strong, so tagging it would ALSO lift the
		// corroboration gate off the weak words beside it.
		{"design systems as a verb", "You will design systems that scale to millions of requests.",
			nil, []string{"design-systems"}},
		{"design systems verb does not lift the gate",
			"Backend engineer: you will design systems and sketch out solutions with the team.",
			nil, []string{"design-systems", "sketch"}},
		{"design systems architecture is still not it",
			"You will design system architecture for our microservices.", nil, []string{"design-systems"}},
		{"research practices", "Run user research and usability testing with our PMs.",
			[]string{"user-research", "usability-testing"}, nil},
		{"craft practices", "Interaction design and typography matter here, alongside Figma.",
			[]string{"interaction-design", "typography", "figma"}, nil},
		{"design thinking", "We practise design thinking end to end.", []string{"design-thinking"}, nil},
		{"motion", "Motion design and motion graphics for product launches.",
			[]string{"motion-design", "motion-graphics"}, nil},
		// CAD / EDA
		{"mechanical cad", "3D modelling in SolidWorks and PTC Creo, drawings in AutoCAD.",
			[]string{"solidworks", "creo", "autocad"}, nil},
		{"more cad", "Experience with CATIA, SketchUp and Autodesk Inventor.",
			[]string{"catia", "sketchup", "autodesk-inventor"}, nil},
		{"eda", "PCB layout in Altium and KiCad; simulation in ANSYS.",
			[]string{"altium", "kicad", "ansys"}, nil},
		{"cad phrases", "Drafting in 3ds Max, Fusion 360 and Civil 3D.",
			[]string{"3ds-max", "fusion-360", "civil-3d"}, nil},
		// Trade and prose collisions that keep whole products out of the dictionary.
		// A carpentry "framer", the Spanish "creo que", and the splined shafts of the
		// very mechanical population this change splits out would each have tagged a
		// posting with a design tool it never mentions — and a false STRONG token also
		// lifts the corroboration gate off every weak word beside it.
		{"framer the carpenter", "Framer needed for residential construction crews.", nil, []string{"framer"}},
		{"framer motion is not the design tool", "Frontend dev: animate with Framer Motion.", nil, []string{"framer"}},
		{"creo in spanish prose", "Creo que este puesto es para ti.", nil, []string{"creo"}},
		{"splined shafts", "Design splined shafts and spline broaching fixtures for gearboxes.", nil, []string{"spline"}},
		// homonyms: uncorroborated prose must stay silent
		{"blender in a kitchen", "Line cook: operate the industrial blender and mixer.", nil, []string{"blender"}},
		{"sketch verb", "You will sketch out ideas with the team each morning.", nil, []string{"sketch"}},
		{"principle noun", "Our guiding principle is respect for the customer.", nil, []string{"principle"}},
		{"eagle-eyed", "We need an eagle-eyed proofreader for our brochures.", nil, []string{"eagle"}},
		{"maya the person", "You will report to Maya, our store manager.", nil, []string{"maya"}},
		// homonyms: corroborated by a strong design token → tagged
		{"sketch corroborated", "Our team designs in Figma and Sketch.", []string{"figma", "sketch"}, nil},
		// Two gated words do NOT corroborate each other — a strong token has to be
		// present, exactly as for the broad concepts.
		{"maya and blender corroborated", "Character rigging in Maya and Blender, textures in Photoshop.",
			[]string{"maya", "blender", "photoshop"}, nil},
		{"gated words alone stay silent", "Sculpting in Maya and Blender.", nil, []string{"maya", "blender"}},
		// A design word that is ALSO ordinary prose cannot be a strong token either —
		// otherwise it corroborates the gated words and the gate is decorative. These
		// four join the gate for that reason, each probed in its real false context.
		{"typography in retail prose", "Retail assistant. Typography of the shelf labels matters; sketch out ideas for displays.",
			nil, []string{"typography", "sketch"}},
		{"illustrator the occupation", "Hiring an Illustrator for our children's book imprint.",
			nil, []string{"illustrator"}},
		{"canva in a store posting", "Store manager. Use Canva for posters. Accessibility of the entrance is your duty.",
			nil, []string{"canva", "accessibility"}},
		{"lottie the given name", "You will report to Lottie, our head of retail operations. Sketch out weekly rotas.",
			nil, []string{"lottie", "sketch"}},
		{"wireframes in cad prose", "CNC operator: read wireframes and 2D drawings before fabrication.",
			nil, []string{"wireframing"}},
		// accessibility is a broad word, so it needs corroboration too
		{"accessibility alone", "An accessibility ramp is available at the entrance.", nil, []string{"accessibility"}},
		{"accessibility corroborated", "Accessibility work in Figma, meeting WCAG 2.2.",
			[]string{"accessibility", "figma", "wcag"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Parse(c.in)
			for _, w := range c.want {
				if !contains(got, w) {
					t.Errorf("Parse(%q) = %v, missing %q", c.in, got, w)
				}
			}
			for _, a := range c.absent {
				if contains(got, a) {
					t.Errorf("Parse(%q) = %v, must NOT contain %q", c.in, got, a)
				}
			}
		})
	}
}

// The tail of the design/CAD vocabulary: the tools and phrases the first batch of
// cases did not reach, kept as their own case list so each alias has a witness.
func TestParse_DesignAndCADVocabTail(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Interactive prototypes in ProtoPie.", "protopie"},
		{"Social assets in Canva, source files in Figma.", "canva"},
		{"Workshops run in FigJam.", "figjam"},
		{"We hold ourselves to a11y standards with Figma.", "accessibility"},
		{"Assemblies modelled in Siemens NX.", "siemens-nx"},
		{"Analog layout in Cadence Virtuoso.", "cadence-virtuoso"},
		{"Parametric modelling in Creo Parametric.", "creo"},
		{"You will run ux research sessions with Figma prototypes.", "user-research"},
		{"You will map user flows before building.", "user-flows"},
		{"Rapid prototyping of new concepts in Figma.", "prototyping"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			got := Parse(c.in)
			for _, g := range got {
				if g == c.want {
					return
				}
			}
			t.Errorf("Parse(%q) = %v, missing %q", c.in, got, c.want)
		})
	}
}

// TestParse_SEOTooling covers the search-optimization toolchain. The corroboration
// case is the point of the block: `seo` is an ambiguousWords member, so it is
// dropped unless the same text carries a strong token. Before these entries a
// posting that named its whole toolchain but no engineering technology corroborated
// nothing and lost even the `seo` tag it obviously deserved.
func TestParse_SEOTooling(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string // must be present
	}{
		{"ahrefs", "Experience with Ahrefs required.", []string{"ahrefs"}},
		{"semrush", "You will own reporting in Semrush.", []string{"semrush"}},
		{"screaming frog", "Run crawls in Screaming Frog.", []string{"screaming-frog"}},
		{"google search console", "Monitor Google Search Console daily.", []string{"google-search-console"}},
		{
			"tooling corroborates the ambiguous seo canonical",
			"SEO Specialist. You will own keyword research in Ahrefs and Semrush.",
			[]string{"ahrefs", "semrush", "seo"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.text)
			for _, w := range tc.want {
				if !slices.Contains(got, w) {
					t.Errorf("Parse(%q) = %v, want it to contain %q", tc.text, got, w)
				}
			}
		})
	}
}

// TestParse_SEOAloneStillDropped pins the other half of the corroboration rule: a
// bare mention of SEO in otherwise non-technical prose must still resolve to
// nothing. The new tooling entries must not weaken that gate.
func TestParse_SEOAloneStillDropped(t *testing.T) {
	got := Parse("A retail assistant with an interest in SEO and social media.")
	if slices.Contains(got, "seo") {
		t.Errorf("Parse(...) = %v, want no bare seo without corroboration", got)
	}
}
