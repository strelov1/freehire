## ADDED Requirements

### Requirement: The panel offers to attach a tailored CV only when one exists and a form can take it

The panel SHALL show an "Attach tailored CV" action for the job currently in view only when
both hold: (1) the caller already has a tailored CV for that job's slug, and (2) the page
the panel is looking at reports a file upload field (the same detection `extractUploads`
already performs for the application-form heuristic). Either condition failing alone SHALL
hide the action rather than show it disabled — a job with no tailored CV yet is directed to
the existing manual tailoring flow, not to a button that would do nothing.

#### Scenario: The action appears when both conditions hold

- **WHEN** the panel is looking at a page with a file upload field, and the caller has a
  tailored CV whose job slug matches the job in view
- **THEN** the panel shows the "Attach tailored CV" action

#### Scenario: No tailored CV for this job hides the action

- **WHEN** the panel is looking at a page with a file upload field, and the caller has no
  tailored CV for the job in view (whether they have tailored CVs for other jobs or none at
  all)
- **THEN** the panel does not show the action

#### Scenario: No file upload field hides the action

- **WHEN** the caller has a tailored CV for the job in view, and the page the panel is
  looking at reports no file upload field
- **THEN** the panel does not show the action

### Requirement: Attaching places the tailored CV's rendered PDF into the field via the debugging protocol

Triggering the action SHALL fetch the matched tailored CV's rendered PDF and place it into
the page's detected file upload field using the Chrome debugging protocol's file-input
primitive, because the field's `files` property carries no script-accessible setter — this
holds for a page's own script and for the extension's content script alike, so no
script-only path exists.

The extension SHALL attach the debugger to the tab, issue the placement call, and detach
immediately afterward, whether the call succeeded or failed — the attached state is
observable to the person as a persistent browser banner, and it SHALL NOT outlive the single
write it exists for.

#### Scenario: A successful attach leaves the debugger detached

- **WHEN** the action places the tailored CV's PDF into the field successfully
- **THEN** the field holds that file and the extension has detached the debugger from the tab

#### Scenario: A failed attach still leaves the debugger detached

- **WHEN** the action's attempt to place the file fails for any reason
- **THEN** the extension detaches the debugger from the tab and reports the failure to the
  caller rather than leaving the field unfilled with no explanation

### Requirement: A file upload field otherwise remains untouched by autofill

Outside this action, the deterministic and agent-driven fillers SHALL continue to treat a
file upload field as a non-fillable control, matching the existing rule this specification
already states for every other question type autofill does not answer. This action does not
change what either filler does with an upload field it did not itself place a file into.

#### Scenario: An unrelated fill run does not touch the upload field

- **WHEN** the agent-driven or deterministic autofill fills the rest of a form's questions
- **THEN** neither filler attempts to write to the form's file upload field
