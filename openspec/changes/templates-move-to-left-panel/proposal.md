## Why

Choosing a template is a decision about how the CV LOOKS, and everything else that decides that
— the font, its size, the page margins — is a tab of the left panel. The template gallery sits
in the right panel instead, among the tabs that MEASURE the document: the job match, the ATS
score, the vacancy it is being tailored to. So changing a font and changing a template, one
question in the user's head, are two panels apart.

The right panel already states its own rule: its tabs are divided by what each one measures,
and none may mix baselines. The template gallery measures nothing. It is the one tab there that
does not answer "how does this document compare", and moving it to where the rest of the
document's appearance lives leaves both panels saying one thing each.

## What Changes

- The template gallery becomes a tab of the LEFT panel, beside the editor, the appearance
  settings and the chat. Picking a template keeps working exactly as it does now — the preview
  re-renders and the choice is saved against the CV.
- The right panel loses its Templates tab and keeps the three that measure the document: the job
  description, the job match and the score. Its stated rule — tabs divided by what they measure
  — now holds without an exception.
- On a narrow viewport the same move applies to the mobile tab strip, so the surface has one
  arrangement rather than one per width.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `tailor-workspace`: the three-column layout names which tabs each side panel holds, so moving
  a tab between them changes the requirement rather than only the markup.

## Impact

- `web/src/routes/tailor/[slug]/+page.svelte` — the left panel's tab set and its mobile strip.
- `web/src/lib/tailor/ArtifactPanel.svelte` — the right panel drops a tab from its `Tab` union
  and its list.
- The template picker component itself is unchanged: it is moved, not rewritten.
- No API change, no stored state change. A CV's template is already a field of the CV.
