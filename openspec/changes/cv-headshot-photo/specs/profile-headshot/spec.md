## ADDED Requirements

### Requirement: One stored headshot per user

The system SHALL let an authenticated user store at most one headshot: uploading a new image replaces the previous one. The image SHALL be held as an object in the configured S3 bucket under a key derived from the authenticated user id — never from client input — with a pointer (object key plus upload time) recorded on the user's row.

#### Scenario: Upload stores the headshot

- **WHEN** a signed-in user uploads an image to the headshot endpoint
- **THEN** the object is stored under that user's derived key and the response reports the headshot as present with its upload time

#### Scenario: A second upload replaces the first

- **WHEN** a user who already has a headshot uploads another image
- **THEN** the stored object is the new image, the upload time advances, and no second object accumulates for that user

### Requirement: An uploaded image is normalized before it is stored

The system SHALL normalize every accepted upload to a fixed square portrait rather than storing the original bytes: it SHALL decode the image, apply the orientation the source file declares, crop to the centred square, scale to a fixed edge length, and re-encode to a single output format. Input SHALL be bounded by a maximum byte size, and any payload that does not decode as a supported image type SHALL be rejected with a client error and stored nowhere.

#### Scenario: A landscape photo becomes a square portrait

- **WHEN** a user uploads a wide photograph
- **THEN** the stored object is the centred square of that photograph at the fixed edge length, in the normalized format

#### Scenario: A rotated phone photo is stored upright

- **WHEN** a user uploads a photograph whose file declares a rotated orientation
- **THEN** the stored image is rotated to match that declaration, so the face is upright in the rendered CV

#### Scenario: A non-image payload is refused

- **WHEN** a user uploads a PDF, an executable, or a corrupt image to the headshot endpoint
- **THEN** the request is rejected with a client error, no object is written, and any previously stored headshot is left intact

#### Scenario: An oversized upload is refused

- **WHEN** a user uploads an image larger than the accepted byte limit
- **THEN** the request is rejected with a client error and nothing is stored

### Requirement: Headshot read and removal are owner-scoped

The system SHALL serve and delete a headshot only for the authenticated owner: the endpoints SHALL derive the object key from the session's user id, so no request can name another user's headshot. Presence and the image SHALL be separate reads — the headshot resource reports whether one is stored and when it was uploaded, and its image sub-resource streams the bytes — because a client needs the former to choose what to render far more often than it needs the latter. Removing a headshot SHALL delete both the object and the pointer, so the user is left as if they had never uploaded.

#### Scenario: Owner fetches their headshot image

- **WHEN** a signed-in user with a stored headshot requests the image sub-resource
- **THEN** the system streams the stored image bytes with the normalized image content type

#### Scenario: Presence is readable without the bytes

- **WHEN** a signed-in user reads the headshot resource
- **THEN** the system reports whether a headshot is stored and, when it is, its upload time — without transferring the image

#### Scenario: No headshot stored

- **WHEN** a signed-in user with no headshot reads the headshot resource and then its image
- **THEN** the read reports the absence as a normal state and the image request reports that none is stored, never another user's image or a server error

#### Scenario: Removal clears object and pointer

- **WHEN** a user removes their headshot
- **THEN** the object is deleted from storage, the pointer on the user's row is cleared, and a subsequent read reports no headshot

### Requirement: The headshot feature degrades when object storage is unconfigured

The system SHALL treat object storage as optional: when it is unconfigured the headshot service SHALL report itself disabled and its upload, image, and removal endpoints SHALL answer `501 Not Implemented`, while every other profile and CV endpoint keeps working. The presence read SHALL still answer, reporting the feature as disabled, so a client can omit the upload control rather than offer one that fails. A CV template that prints a photo SHALL still render in that state, using the placeholder rather than failing.

#### Scenario: Storage unconfigured

- **WHEN** object storage is not configured and a user uploads, fetches, or removes a headshot
- **THEN** the endpoint returns `501` and the rest of the profile and CV surface is unaffected

#### Scenario: The client can tell the feature is off

- **WHEN** object storage is not configured and a user reads the headshot resource
- **THEN** the read succeeds and reports the feature as disabled with no headshot present

### Requirement: The headshot is not part of the CV document

The system SHALL keep the headshot out of the CV `Document`: the image SHALL be a profile-level asset referenced by the renderer at render time, not a field a client can write through the CV endpoints. Consequently the image SHALL NOT be included in any prompt sent to a language model, and a CV document round-tripped through tailoring SHALL carry no photo data.

#### Scenario: Photo cannot be written through the CV document

- **WHEN** a client submits a CV document containing photo fields or a photo flag
- **THEN** those values are not persisted on the document and do not affect what the renderer prints

#### Scenario: Tailoring never sees the image

- **WHEN** a CV using a photo-bearing template is tailored by the assistant
- **THEN** no image data is part of the model's input, and the stored headshot is unchanged by the run
