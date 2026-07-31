// portrait: a two-column CV with a headshot. The narrow left column opens with the photo
// and continues with contact, skills, languages, and links; the wide right column holds
// the name, summary, experience, education, and projects. Serif (Libertinus), no color —
// black text, a light grey rule, and the grey of the placeholder. NOT ATS-safe: a scanner
// that linearizes the page may interleave the columns, and a portrait is exactly what the
// single-column contract excludes. The text layer stays fully extractable.
//
// The renderer stages photo.jpg beside data.json ONLY when the member has a headshot, and
// says so via has_photo — image() on a missing file is a compile error, so the flag is
// what makes the placeholder path safe. Helpers are duplicated per template because the
// sandbox stages no shared module for #import to resolve.

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
#let daterange(a, b) = if a != "" and b != "" { a + " – " + b } else { a + b }

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

// A bold uppercase section label with a thin rule beneath it.
#let section(t) = {
  v(0.55em)
  text(weight: "bold", size: (9.5 / 9.5) * 1em)[#upper(t)]
  v(0.08em)
  line(length: 100%, stroke: 0.5pt + rgb("#b3b3b3"))
  v(0.2em)
}

// The stand-in shown when no headshot is stored: a head and shoulders drawn from two
// shapes, clipped by the frame. Drawn rather than staged as an asset so it costs nothing
// in the committed SVG previews and needs no file in the sandbox.
#let silhouette(size) = box(
  width: size, height: size, radius: 2pt, clip: true, fill: rgb("#eeeeee"),
  {
    place(center + horizon, dy: -0.14 * size, circle(radius: 0.18 * size, fill: rgb("#c9c9c9")))
    place(center + bottom, dy: 0.12 * size, ellipse(width: 0.72 * size, height: 0.5 * size, fill: rgb("#c9c9c9")))
  },
)

// The photo frame: the stored headshot, or the placeholder. The image is already a
// normalized square, so "cover" only guards against a frame that is not.
#let portrait(size) = if cv.at("has_photo", default: false) {
  box(width: size, height: size, radius: 2pt, clip: true, image("photo.jpg", width: size, height: size, fit: "cover"))
} else {
  silhouette(size)
}

// ---- Full-width header ----
#let hd = cv.header
#{
  set par(justify: false)
  text(weight: "bold", size: (17 / 9.5) * 1em)[#s(hd, "full_name")]
  let summary = s(cv, "summary")
  if summary != "" { linebreak(); v(0.15em); text(style: "italic")[#summary] }
}
#v(0.25em)
#line(length: 100%, stroke: 0.7pt + rgb("#333333"))
#v(0.3em)

// ---- Left sidebar content ----
#let sidebar = {
  set par(justify: false)
  // The frame fills the sidebar, whose width follows the page margins — so it is measured
  // at layout time rather than fixed: a ratio cannot be multiplied into a shape's radius.
  // Capped at passport-photo scale, because a portrait that grows with the page margins
  // ends up eating the column the contacts and skills need.
  layout(sz => portrait(calc.min(sz.width, 42mm)))
  v(0.4em)
  // Contact
  let contactLines = ()
  for k in ("email", "phone", "location") {
    let v = s(hd, k)
    if v != "" { contactLines.push(v) }
  }
  if contactLines.len() > 0 {
    section("Contact")
    for l in contactLines { l; linebreak() }
  }
  // Links
  let links = arr(hd, "links")
  if links.filter(l => l != "").len() > 0 {
    section("Links")
    for (i, l) in links.enumerate() {
      if l != "" { link(hrefAt("header", i, l))[#l]; linebreak() }
    }
  }
  // Skills
  let allSkills = arr(cv, "skills").map(g => arr(g, "items")).flatten()
  if allSkills.len() > 0 {
    section("Skills")
    allSkills.join(", ")
  }
  // Languages
  let langs = arr(cv, "languages").map(l => {
    let n = s(l, "name")
    let lv = s(l, "level")
    if lv != "" { n + " (" + lv + ")" } else { n }
  }).filter(n => n != "")
  if langs.len() > 0 {
    section("Languages")
    langs.join(", ")
  }
  // Certifications
  let certs = arr(cv, "certifications").map(c => {
    let name = s(c, "name")
    let issuer = s(c, "issuer")
    let year = s(c, "year")
    let line = name
    if issuer != "" { line = line + " — " + issuer }
    if year != "" { line = line + " (" + year + ")" }
    line
  }).filter(l => l != "")
  if certs.len() > 0 {
    section("Certifications")
    for c in certs { c; linebreak() }
  }
}

// ---- Right main content ----
#let main = {
  // Experience
  let exp = arr(cv, "experience")
  if exp.len() > 0 {
    section("Experience")
    for e in exp {
      set par(justify: false)
      let head = s(e, "company")
      for p in (s(e, "location"), s(e, "role")) {
        if p != "" { head = if head != "" { head + " | " + p } else { p } }
      }
      let dr = daterange(s(e, "start"), s(e, "end"))
      block(above: 0.5em, below: 0.3em)[
        #text(weight: "bold")[#head]
        #if dr != "" { h(1fr); text(fill: rgb("#555555"))[#dr] }
      ]
      let sum = s(e, "summary")
      if sum != "" { par(justify: true)[#sum] }
      let bl = arr(e, "bullets")
      if bl.len() > 0 { list(..bl.map(b => [#b])) }
      let st = arr(e, "stack")
      if st.len() > 0 { par(justify: false)[#text(weight: "bold")[Stack:] #st.join(", ")] }
      v(0.35em)
    }
  }
  // Education
  let edu = arr(cv, "education")
  if edu.len() > 0 {
    section("Education")
    set par(justify: false)
    for ed in edu {
      let deg = s(ed, "degree")
      let field = s(ed, "field")
      if field != "" { deg = if deg != "" { deg + ", " + field } else { field } }
      let inst = s(ed, "institution")
      let ln = if deg != "" and inst != "" { deg + " | " + inst } else { deg + inst }
      let dr = daterange(s(ed, "start"), s(ed, "end"))
      block(above: 0.2em)[#ln#if dr != "" { h(1fr); text(fill: rgb("#555555"))[#dr] }]
    }
  }
  // Projects
  let projects = arr(cv, "projects")
  if projects.len() > 0 {
    section("Projects")
    list(..projects.enumerate().map(entry => {
      let (i, p) = entry
      let name = s(p, "name")
      let lnk = s(p, "link")
      let bl = arr(p, "bullets")
      [#text(weight: "bold")[#name]#if bl.len() > 0 [: #bl.join(" ")]#if lnk != "" [ (#link(hrefAt("projects", i, lnk))[#lnk])]]
    }))
  }
}

#grid(
  columns: (32%, 1fr),
  gutter: 1.1em,
  sidebar,
  main,
)
