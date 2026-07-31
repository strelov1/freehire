## ADDED Requirements

### Requirement: Photo-bearing templates compose the stored headshot

The registry SHALL mark which templates print a headshot, and rendering such a template SHALL compose the owner's stored headshot into the PDF. The image SHALL reach the renderer as bytes staged alongside the document, not as a URL the rendering engine fetches, so rendering stays sandboxed and makes no outbound request. A template not marked as photo-bearing SHALL render identically to before, and the system SHALL NOT read object storage for it.

#### Scenario: A photo-bearing template prints the headshot

- **WHEN** a user with a stored headshot renders a CV whose template is photo-bearing
- **THEN** the PDF contains that image, and the CV's text layer remains extractable

#### Scenario: A photoless template costs no storage read

- **WHEN** a CV using a template that is not photo-bearing is rendered
- **THEN** the headshot is not fetched from object storage and the output is unchanged from a render made without any stored headshot

### Requirement: A photo-bearing template without a headshot renders a placeholder

The system SHALL render a neutral silhouette placeholder in the photo frame when a photo-bearing template is used by a candidate who has no stored headshot, rather than failing the render or leaving an empty gap. The placeholder SHALL be drawn by the template itself, so it needs no stored asset and appears identically in the generated gallery thumbnails.

#### Scenario: No headshot uploaded

- **WHEN** a user with no stored headshot renders a CV using a photo-bearing template
- **THEN** the render succeeds and the photo frame shows the silhouette placeholder

#### Scenario: Gallery thumbnail shows the photo frame

- **WHEN** the committed preview thumbnails are regenerated from the sample document, which carries no headshot
- **THEN** each photo-bearing template's thumbnail shows its photo frame with the placeholder, so the gallery represents the template's actual layout

## MODIFIED Requirements

### Requirement: Available CV templates are discoverable via the API

The system SHALL expose the registered CV templates over a read endpoint so clients can list the available templates without hard-coding them. Each entry SHALL include the template `id`, a human-facing `label`, a short style descriptor, an `ats_safe` boolean indicating whether the template follows the ATS single-column contract, and a `photo` boolean indicating whether the template prints a headshot. The endpoint SHALL be available to any authenticated user allowed to use the CV builder.

#### Scenario: List available templates

- **WHEN** an authorized user requests the CV templates list endpoint
- **THEN** the system returns every registered template with its `id`, `label`, style descriptor, `ats_safe` flag, and `photo` flag, including `classic-ats` marked as ATS-safe and photoless, and `sidebar` marked as not ATS-safe

#### Scenario: Photo-bearing templates are identifiable

- **WHEN** an authorized user lists the CV templates
- **THEN** the photo-bearing templates are returned with `photo` true and `ats_safe` false, so a client can prompt for a headshot upload before the template is used
