// sidebar: a two-column CV. A full-width header (name + summary) sits above a grid whose
// narrow left column holds contact, skills, languages, and links, and whose wide right column
// holds experience, education, and projects. Serif (Libertinus), no color — black text and a
// light grey rule only. NOT ATS-safe: a scanner that linearizes the page left-to-right may
// interleave the two columns, so this template is flagged accordingly in the UI. The text
// layer is still fully extractable. Reads the CV from data.json written by the renderer.

#let cv = json("data.json")
#let s(d, k) = d.at(k, default: "")
#let arr(d, k) = d.at(k, default: ())
#let daterange(a, b) = if a != "" and b != "" { a + " – " + b } else { a + b }

#set document(title: s(cv.header, "full_name"))
// Per-side page margins (inches) from the document; any missing or non-positive side
// falls back to 0.5in, so an unsanitized/sample document still renders sanely.
#let mg(k) = {
  let v = cv.at("margins", default: (:)).at(k, default: 0)
  (if v > 0 { v } else { 0.5 }) * 1in
}
#set page(paper: "a4", margin: (left: mg("left"), right: mg("right"), top: mg("top"), bottom: mg("bottom")))
#set text(font: "Libertinus Serif", size: 9.5pt)
#set par(leading: 0.5em, justify: true)

// A bold uppercase section label with a thin rule beneath it.
#let section(t) = {
  v(0.55em)
  text(weight: "bold", size: 9.5pt)[#upper(t)]
  v(0.08em)
  line(length: 100%, stroke: 0.5pt + rgb("#b3b3b3"))
  v(0.2em)
}

// ---- Full-width header ----
#let hd = cv.header
#{
  set par(justify: false)
  text(weight: "bold", size: 17pt)[#s(hd, "full_name")]
  let summary = s(cv, "summary")
  if summary != "" { linebreak(); v(0.15em); text(style: "italic")[#summary] }
}
#v(0.25em)
#line(length: 100%, stroke: 0.7pt + rgb("#333333"))
#v(0.3em)

// ---- Left sidebar content ----
#let sidebar = {
  set par(justify: false)
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
  let links = arr(hd, "links").filter(l => l != "")
  if links.len() > 0 {
    section("Links")
    for l in links { l; linebreak() }
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
    list(..projects.map(p => {
      let name = s(p, "name")
      let lnk = s(p, "link")
      let bl = arr(p, "bullets")
      [#text(weight: "bold")[#name]#if bl.len() > 0 [: #bl.join(" ")]#if lnk != "" [ (#lnk)]]
    }))
  }
}

#grid(
  columns: (32%, 1fr),
  gutter: 1.1em,
  sidebar,
  main,
)
