## ADDED Requirements

### Requirement: The transcript follows the stream only while the reader is at the bottom

The chat transcript SHALL scroll to the newest content on an arriving turn event only while
the reader is already at the bottom of the pane. Once the reader has scrolled away from the
bottom, arriving events MUST leave the scroll position untouched until the reader returns.

"At the bottom" is a tolerance, not an equality: a pane within a small distance of its own
end counts as at the bottom, because the last line of a streaming answer grows under the
reader and an exact test would drop out of following on its own.

Returning to the bottom by scrolling MUST resume following without any further action.

#### Scenario: A reader at the bottom keeps seeing the newest text

- **WHEN** the pane is scrolled to its end and a turn event appends text
- **THEN** the pane scrolls so the new text is visible

#### Scenario: A reader who scrolled up is left alone

- **WHEN** the reader has scrolled up beyond the tolerance and turn events keep arriving
- **THEN** the scroll position does not change

#### Scenario: Scrolling back to the bottom resumes following

- **WHEN** a reader who had scrolled up scrolls back to the end of the pane
- **AND** a further turn event appends text
- **THEN** the pane scrolls to the new text again

### Requirement: Deliberate acts return the reader to the bottom

Sending a message, starting an unattended run, and opening a conversation SHALL scroll the
pane to the bottom regardless of where the reader was, and resume following. These are acts
the reader performed, not frames that arrived; leaving them off screen would hide the very
thing the act produced.

#### Scenario: Sending from a scrolled-up position

- **WHEN** the reader has scrolled up and submits a message
- **THEN** the pane scrolls to the bottom and follows the answer as it streams

#### Scenario: Opening a conversation shows its end

- **WHEN** a stored conversation is opened
- **THEN** the pane is positioned at the most recent message

### Requirement: A control returns the reader to the latest content

While the pane is not following, the transcript SHALL offer a visible control that scrolls to
the bottom and resumes following. The control MUST be absent while the pane is following, so
it never covers content it is not needed for.

#### Scenario: The control appears once following stops

- **WHEN** the reader scrolls up beyond the tolerance
- **THEN** a "jump to latest" control is shown

#### Scenario: The control restores following

- **WHEN** the reader activates that control
- **THEN** the pane scrolls to the bottom, the control disappears, and later events scroll the pane again
