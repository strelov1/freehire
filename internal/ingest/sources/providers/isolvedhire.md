**ApplicantPro / iSolved Hire traps** (`isolvedhire`):

- **A gone board answers 200 at `sitemap.xml`** with `This career site has been disabled.` or `You may have typed the url for this website incorrectly.`, which surfaced in `board_health` as an XML syntax error on line 3 until an audit went looking. The adapter wraps `sources.ErrBoardGone` on a stable fragment of those notices.
