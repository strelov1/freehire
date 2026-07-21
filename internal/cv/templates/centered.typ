// centered: a single-column, ATS-safe CV with a centered header. Name large and centered,
// contacts centered on one line under it, a centered summary, and centered section headings
// with a full-width rule; entry content stays left-aligned for readability. No color —
// black text and a light grey rule only. Reads the CV from data.json (written next to it by
// the renderer) and uses Libertinus Serif (embedded in the typst binary) for a clean text
// layer identical locally and in the distroless image.

#let cv = json("data.json")
#let s(d, k) = d.at(k, default: "")
#let arr(d, k) = d.at(k, default: ())
#let daterange(a, b) = if a != "" and b != "" { a + " – " + b } else { a + b }

#set document(title: s(cv.header, "full_name"))
#set page(paper: "a4", margin: (x: 1.6cm, top: 1.2cm, bottom: 1.2cm))
#set text(font: "Libertinus Serif", size: 9.5pt)
#set par(leading: 0.5em, justify: true)

// A centered section heading with a rule beneath it.
#let section(t) = {
  v(0.7em)
  align(center)[#text(weight: "bold", size: 10pt)[#upper(t)]]
  v(0.1em)
  line(length: 100%, stroke: 0.5pt + rgb("#b3b3b3"))
  v(0.25em)
}

// ---- Header: centered name, contacts, and summary ----
#let hd = cv.header
#let contacts = {
  let parts = ()
  for k in ("phone", "email", "location") {
    let v = s(hd, k)
    if v != "" { parts.push(v) }
  }
  for l in arr(hd, "links") {
    if l != "" { parts.push(l) }
  }
  parts
}
#align(center)[
  #text(weight: "bold", size: 18pt)[#s(hd, "full_name")]
  #if contacts.len() > 0 { linebreak(); text(size: 9pt)[#contacts.join("  |  ")] }
  #{
    let summary = s(cv, "summary")
    if summary != "" { linebreak(); v(0.2em); text(style: "italic")[#summary] }
  }
]

// ---- Experience ----
#let exp = arr(cv, "experience")
#if exp.len() > 0 {
  section("Experience")
  for e in exp {
    set par(justify: false)
    let head = s(e, "company")
    for p in (s(e, "location"), s(e, "role")) {
      if p != "" { head = if head != "" { head + " | " + p } else { p } }
    }
    let dr = daterange(s(e, "start"), s(e, "end"))
    block(above: 0.55em, below: 0.4em)[
      #text(weight: "bold")[#head]
      #if dr != "" { h(1fr); text(fill: rgb("#555555"))[#dr] }
    ]
    let sum = s(e, "summary")
    if sum != "" { par(justify: true)[#sum] }
    let bl = arr(e, "bullets")
    if bl.len() > 0 { list(..bl.map(b => [#b])) }
    let st = arr(e, "stack")
    if st.len() > 0 { par(justify: false)[#text(weight: "bold")[Stack:] #st.join(", ")] }
    v(0.4em)
  }
}

// ---- Projects ----
#let projects = arr(cv, "projects")
#if projects.len() > 0 {
  section("Projects")
  list(..projects.map(p => {
    let name = s(p, "name")
    let lnk = s(p, "link")
    let bl = arr(p, "bullets")
    [#text(weight: "bold")[#name]#if bl.len() > 0 [: #bl.join(" ")]#if lnk != "" [ (#lnk)]]
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
  align(center)[#allSkills.join("  ·  ")]
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
    align(center)[#names.join("  ·  ")]
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
    align(center)[#items.join(";  ")]
  }
}
