// classic-ats: a single-column, ATS-safe CV. Compact, information-dense layout:
// name + contacts on one line, "Company | Location | Title (dates)" role headers with a
// context line, bullet achievements, and a per-role Stack line; Education / Skills /
// Languages render inline. Reads the CV from data.json (written next to it by the
// renderer). Uses Libertinus Serif (embedded in the typst binary) so the text layer
// extracts cleanly and rendering is identical locally and in the distroless image.

#let cv = json("data.json")
#let s(d, k) = d.at(k, default: "")
#let arr(d, k) = d.at(k, default: ())
#let linkHrefs = cv.at("link_hrefs", default: (:))
// Where a link at this position should point. The payload resolves every link to an absolute
// URL, and to our own redirect when the candidate has tracing on; the printed text is the
// candidate's own either way, so a missing entry falls back to it.
#let hrefAt(kind, i, shown) = {
  let a = linkHrefs.at(kind, default: ())
  if i < a.len() and a.at(i) != "" { a.at(i) } else { shown }
}

#set document(title: s(cv.header, "full_name"))
// Per-side page margins (inches) from the document; any missing or non-positive side
// falls back to 0.5in, so an unsanitized/sample document still renders sanely.
#let mg(k) = {
  let v = cv.at("margins", default: (:)).at(k, default: 0)
  (if v > 0 { v } else { 0.5 }) * 1in
}
#set page(paper: "a4", margin: (left: mg("left"), right: mg("right"), top: mg("top"), bottom: mg("bottom")))
// Typography from the document's style block. Every value is optional and a zero/empty one
// falls back to this template's own, so a CV that sets nothing renders exactly as it always
// did. These helpers are duplicated per template on purpose: the renderer stages only
// template.typ, so an #import of a shared module would not resolve.
#let st = cv.at("style", default: (:))
#let ty(k, fallback) = {
  let v = st.at(k, default: 0)
  if v > 0 { v } else { fallback }
}
#let fontFamily = {
  let f = st.at("font_family", default: "")
  if f != "" { f } else { "Libertinus Serif" }
}
#set text(font: fontFamily, size: ty("font_size", 9.5) * 1pt)
#set par(leading: ty("line_height", 0.5) * 1em, justify: true)
#show link: set text(fill: rgb("#2b6cb0"))

// A horizontal separator rule between blocks.
#let rule = { v(0.12em); line(length: 100%, stroke: 0.5pt + rgb("#b3b3b3")); v(0.06em) }
// A bold uppercase section label.
#let sectionLabel(t) = text(weight: "bold", size: (9.5 / 9.5) * 1em)[#upper(t)]
// A free-form date range as written (no parsing).
#let daterange(a, b) = if a != "" and b != "" { a + " – " + b } else { a + b }

// ---- Header: name + contacts on one line, tagline below ----
#let hd = cv.header
#let contacts = {
  let parts = ()
  for k in ("phone", "email", "location") {
    let v = s(hd, k)
    if v != "" { parts.push([#v]) }
  }
  for (i, l) in arr(hd, "links").enumerate() {
    if l != "" { parts.push(link(hrefAt("header", i, l))[#l]) }
  }
  parts
}
#{
  set par(justify: false)
  [#text(weight: "bold", size: (12 / 9.5) * 1em)[#s(hd, "full_name")]]
  if contacts.len() > 0 { [ | ]; contacts.join([ | ]) }
  let summary = s(cv, "summary")
  if summary != "" { linebreak(); summary }
}
#rule

// ---- Experience ----
#let exp = arr(cv, "experience")
#if exp.len() > 0 {
  sectionLabel("Experience")
  for e in exp {
    set par(justify: false)
    // Role header: **Company | Location | Title (dates)**
    let head = s(e, "company")
    for p in (s(e, "location"), s(e, "role")) {
      if p != "" { head = if head != "" { head + " | " + p } else { p } }
    }
    let dr = daterange(s(e, "start"), s(e, "end"))
    if dr != "" { head = head + " (" + dr + ")" }
    block(above: 0.7em, below: 0.45em)[#text(weight: "bold")[#head]]
    // Context line.
    let sum = s(e, "summary")
    if sum != "" { par(justify: true)[#sum] }
    // Achievement bullets.
    let bl = arr(e, "bullets")
    if bl.len() > 0 { list(..bl.map(b => [#b])) }
    // Stack line.
    let st = arr(e, "stack")
    if st.len() > 0 { par(justify: false)[#text(weight: "bold")[Stack:] #st.join(", ")] }
    v(0.55em)
  }
}

// ---- Projects ----
#let projects = arr(cv, "projects")
#if projects.len() > 0 {
  sectionLabel("Projects")
  list(..projects.enumerate().map(entry => {
    let (i, p) = entry
    let name = s(p, "name")
    let lnk = s(p, "link")
    let bl = arr(p, "bullets")
    [#text(weight: "bold")[#name]#if bl.len() > 0 [: #bl.join(" ")]#if lnk != "" [ (#link(hrefAt("projects", i, lnk))[#lnk])]]
  }))
  rule
}

// ---- Education (inline) ----
#let edu = arr(cv, "education")
#if edu.len() > 0 {
  set par(justify: false)
  let entries = edu.map(ed => {
    let deg = s(ed, "degree")
    let field = s(ed, "field")
    if field != "" { deg = if deg != "" { deg + ", " + field } else { field } }
    let inst = s(ed, "institution")
    let line = if deg != "" and inst != "" { deg + " | " + inst } else { deg + inst }
    let dr = daterange(s(ed, "start"), s(ed, "end"))
    if dr != "" { line = line + " (" + dr + ")" }
    line
  })
  block(above: 0.5em, below: 0.3em)[#sectionLabel("Education")#h(1.2em)#entries.join("; ")]
}

// ---- Skills (inline, flattened) ----
#let allSkills = arr(cv, "skills").map(g => arr(g, "items")).flatten()
#if allSkills.len() > 0 {
  block(above: 0.4em, below: 0.3em)[#text(weight: "bold")[SKILLS:] #allSkills.join(", ")]
}

// ---- Languages (inline) ----
#let langs = arr(cv, "languages")
#if langs.len() > 0 {
  let names = langs.map(l => {
    let n = s(l, "name")
    let lv = s(l, "level")
    if lv != "" { n + " (" + lv + ")" } else { n }
  }).filter(n => n != "")
  if names.len() > 0 {
    set par(justify: false)
    block(above: 0.3em)[#text(weight: "bold")[LANGUAGES:] #names.join(", ")]
  }
}

// ---- Certifications (inline, optional) ----
#let certs = arr(cv, "certifications")
#if certs.len() > 0 {
  let items = certs.map(c => {
    let name = s(c, "name")
    let issuer = s(c, "issuer")
    let year = s(c, "year")
    let line = name
    if issuer != "" { line = line + " — " + issuer }
    if year != "" { line = line + " (" + year + ")" }
    line
  }).filter(l => l != "")
  if items.len() > 0 {
    set par(justify: false)
    block(above: 0.3em)[#text(weight: "bold")[CERTIFICATIONS:] #items.join("; ")]
  }
}
