// timeline: a single-column, ATS-safe CV. Experience entries run down a vertical rail — a
// left border rule with a small accent dot per entry — but the rail is pure decoration (a box
// stroke and a placed circle carry no text), so the underlying content is still one linear
// top-to-bottom text stream, read exactly like classic-ats. Serif (Libertinus Serif, embedded
// in the typst binary). Reads the CV from data.json (written next to it by the renderer).

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

#let accent = rgb("#2b6cb0")
#show link: set text(fill: accent)

// A bold uppercase section label with a thin rule beneath it.
#let section(t) = {
  v(0.7em)
  text(weight: "bold", size: (9.5 / 9.5) * 1em, tracking: (0.5 / 9.5) * 1em)[#upper(t)]
  v(0.1em)
  line(length: 100%, stroke: 0.5pt + rgb("#b3b3b3"))
  v(0.3em)
}

// One rail entry: a left border rule with a small filled dot at its top holding body. The
// stroke and the placed circle are graphics, not text, so extraction order is unaffected —
// content inside still reads top to bottom like any other template's entry.
#let rail(body) = box(
  width: 100%,
  inset: (left: 1em, bottom: 0.15em),
  stroke: (left: 1.2pt + rgb("#c7d7ea")),
)[
  #place(top + left, dx: -1.05em, dy: 0.15em)[#box(width: 0.5em, height: 0.5em, radius: 0.25em, fill: accent)]
  #body
]

// ---- Header ----
#let hd = cv.header
#let contacts = {
  let parts = ()
  for k in ("phone", "email", "location") {
    let v = s(hd, k)
    if v != "" { parts.push(v) }
  }
  for (i, l) in arr(hd, "links").enumerate() {
    if l != "" { parts.push(link(hrefAt("header", i, l))[#l]) }
  }
  parts
}
#{
  set par(justify: false)
  text(weight: "bold", size: (18 / 9.5) * 1em)[#s(hd, "full_name")]
  let summary = s(cv, "summary")
  if summary != "" { linebreak(); v(0.15em); text(fill: rgb("#333333"))[#summary] }
  if contacts.len() > 0 { linebreak(); v(0.15em); text(size: (9 / 9.5) * 1em, fill: rgb("#555555"))[#contacts.join("  ·  ")] }
}
#v(0.25em)
#line(length: 100%, stroke: 0.8pt + accent)

// ---- Experience (rail) ----
#let exp = arr(cv, "experience")
#if exp.len() > 0 {
  section("Experience")
  for e in exp {
    rail({
      set par(justify: false)
      let head = s(e, "company")
      for p in (s(e, "location"), s(e, "role")) {
        if p != "" { head = if head != "" { head + " | " + p } else { p } }
      }
      let dr = daterange(s(e, "start"), s(e, "end"))
      [#text(weight: "bold")[#head]#if dr != "" { h(1fr); text(fill: rgb("#555555"))[#dr] }]
      let sum = s(e, "summary")
      if sum != "" { par(justify: true)[#sum] }
      let bl = arr(e, "bullets")
      if bl.len() > 0 { list(..bl.map(b => [#b])) }
      let stk = arr(e, "stack")
      if stk.len() > 0 { par(justify: false)[#text(weight: "bold")[Stack:] #stk.join(", ")] }
    })
    v(0.55em)
  }
}

// ---- Projects ----
#let projects = arr(cv, "projects")
#if projects.len() > 0 {
  section("Projects")
  list(..projects.enumerate().map(entry => {
    let (i, p) = entry
    let name = s(p, "name")
    let lnk = s(p, "link")
    let bl = arr(p, "bullets")
    [#text(weight: "bold")[#name]#if bl.len() > 0 [: #bl.join(" ")]#if lnk != "" [ (#link(hrefAt("projects", i, lnk))[#lnk])]]
  }))
}

// ---- Education ----
#let edu = arr(cv, "education")
#if edu.len() > 0 {
  section("Education")
  set par(justify: false)
  for ed in edu {
    let deg = s(ed, "degree")
    let field = s(ed, "field")
    if field != "" { deg = if deg != "" { deg + ", " + field } else { field } }
    let inst = s(ed, "institution")
    let line = if deg != "" and inst != "" { deg + " | " + inst } else { deg + inst }
    let dr = daterange(s(ed, "start"), s(ed, "end"))
    block(above: 0.2em)[#line#if dr != "" { h(1fr); text(fill: rgb("#555555"))[#dr] }]
  }
}

// ---- Skills (inline, flattened) ----
#let allSkills = arr(cv, "skills").map(g => arr(g, "items")).flatten()
#if allSkills.len() > 0 {
  section("Skills")
  allSkills.join("  ·  ")
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
    section("Languages")
    names.join("  ·  ")
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
    section("Certifications")
    items.join(";  ")
  }
}
