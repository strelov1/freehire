package handler

import (
	"errors"
	"io"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/headshot"
)

// photoHandlers serve the member's headshot: the one image the CV templates that print a
// portrait compose in. Cookie-only throughout — the image is PII, and its only readers are
// the owner's browser and this server's own renderer, so there is no key-authenticated
// path and no public URL.
type photoHandlers struct {
	photos *headshot.Store
}

func newPhotoHandlers(photos *headshot.Store) *photoHandlers {
	return &photoHandlers{photos: photos}
}

func (h *photoHandlers) register(api fiber.Router, mw middleware) {
	api.Put("/me/photo", mw.cookie, h.PutPhoto)
	api.Get("/me/photo", mw.cookie, h.GetPhoto)
	api.Delete("/me/photo", mw.cookie, h.DeletePhoto)
}

// photoMetaResponse is the wire shape of the headshot's presence, returned by the write
// paths so the SPA can swap its preview without a follow-up read. uploaded_at doubles as
// the cache buster on the image URL.
type photoMetaResponse struct {
	Present    bool       `json:"present"`
	UploadedAt *time.Time `json:"uploaded_at,omitempty"`
}

func newPhotoMeta(m headshot.Meta) photoMetaResponse {
	return photoMetaResponse{Present: m.Present, UploadedAt: m.UploadedAt}
}

// PutPhoto stores (or replaces) the caller's headshot from a multipart "file" part. The
// bytes are normalized before they are stored, so an undecodable or oversized upload is a
// 400 and leaves any existing headshot untouched.
func (h *photoHandlers) PutPhoto(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	data, err := readPhotoUpload(c)
	if err != nil {
		return err
	}
	meta, err := h.photos.Put(c.Context(), userID, data)
	if err != nil {
		return mapPhotoError(err)
	}
	return c.JSON(fiber.Map{"data": newPhotoMeta(meta)})
}

// GetPhoto streams the caller's stored headshot. The key is derived from the session, so
// there is no id to scope: a member can only ever ask for their own.
func (h *photoHandlers) GetPhoto(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	data, err := h.photos.Get(c.Context(), userID)
	if err != nil {
		return mapPhotoError(err)
	}
	c.Set(fiber.HeaderContentType, "image/jpeg")
	// Private: the response is one member's face, and a shared cache must not hold it.
	// The SPA busts its own cache with ?v=<uploaded_at>, so a short lifetime is safe and
	// keeps a CV preview from re-fetching the image on every keystroke.
	c.Set(fiber.HeaderCacheControl, "private, max-age=60")
	return c.Send(data)
}

// DeletePhoto removes the caller's headshot (object + pointer).
func (h *photoHandlers) DeletePhoto(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.photos.Delete(c.Context(), userID); err != nil {
		return mapPhotoError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// maxPhotoUpload bounds what is read from the request part. The service applies the same
// ceiling to the decoded image; this one stops a body from being buffered at all.
const maxPhotoUpload = 8 << 20

// readPhotoUpload reads the "file" part into memory. Only multipart is accepted: unlike
// the résumé there is no paste path for an image.
func readPhotoUpload(c *fiber.Ctx) ([]byte, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "missing photo file")
	}
	if fh.Size > maxPhotoUpload {
		return nil, fiber.NewError(fiber.StatusBadRequest, "photo is too large")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "cannot read photo file")
	}
	defer f.Close()
	// LimitReader guards the read itself: fh.Size is the client's declaration, not a
	// promise about what the stream delivers.
	data, err := io.ReadAll(io.LimitReader(f, maxPhotoUpload+1))
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, "cannot read photo file")
	}
	if len(data) > maxPhotoUpload {
		return nil, fiber.NewError(fiber.StatusBadRequest, "photo is too large")
	}
	return data, nil
}

// mapPhotoError renders the service's errors: an unconfigured bucket is 501 (the feature
// is absent, not broken), a bad image is 400, and a missing headshot is 404 — the SPA and
// the renderer both read that as "there is no photo" rather than as a failure.
func mapPhotoError(err error) error {
	switch {
	case errors.Is(err, headshot.ErrStorageDisabled):
		return fiber.NewError(fiber.StatusNotImplemented, "photo storage is not available")
	case errors.Is(err, headshot.ErrNotStored):
		return fiber.NewError(fiber.StatusNotFound, "no photo stored")
	case errors.Is(err, headshot.ErrUnsupportedImage):
		return fiber.NewError(fiber.StatusBadRequest, "the file is not a supported image (JPEG, PNG, or WebP)")
	case errors.Is(err, headshot.ErrTooLarge):
		return fiber.NewError(fiber.StatusBadRequest, "the image is too large")
	default:
		return err
	}
}
