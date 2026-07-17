package cv

import (
	"errors"
	"fmt"
)

// This file holds the CV patch wire shape (Patch + PatchOp) and the pure Apply transform.
// Like cv.go it stays dependency-light so cmd/gen-contracts can emit the TypeScript type,
// and it is server-agnostic (no I/O) — the caller Sanitizes the result before persisting.

// ErrInvalidPatch marks a patch that addresses a field or index that does not exist, or is
// otherwise malformed. Handlers map it to a 422. Apply never mutates the document when it
// returns ErrInvalidPatch — bad addressing is a no-op, not a partial edit.
var ErrInvalidPatch = errors.New("cv: invalid patch")

// PatchOp is the discriminator selecting which field-level edit a Patch performs.
type PatchOp string

const (
	PatchSetSummary     PatchOp = "set_summary"
	PatchSetHeaderField PatchOp = "set_header_field"
	PatchAddBullet      PatchOp = "add_bullet"
	PatchReplaceBullet  PatchOp = "replace_bullet"
	PatchRemoveBullet   PatchOp = "remove_bullet"
	PatchReorderBullets PatchOp = "reorder_bullets"
	PatchSetSkillGroup  PatchOp = "set_skill_group"
)

// Patch is one field-level edit to a CV Document. Op selects the operation; the remaining
// fields are its address and payload, and only the ones an op needs are read. A patch names
// the single field it changes rather than re-emitting the document, so an LLM tailoring a CV
// mid-session cannot silently drop untouched sections.
type Patch struct {
	Op         PatchOp  `json:"op"`
	Experience int      `json:"experience,omitempty"` // index into Document.Experience
	Bullet     int      `json:"bullet,omitempty"`     // index into the entry's Bullets (replace/remove)
	Field      string   `json:"field,omitempty"`      // header field name (set_header_field)
	Value      string   `json:"value,omitempty"`      // summary / bullet / header value
	Order      []int    `json:"order,omitempty"`      // permutation of bullet indices (reorder_bullets)
	Group      string   `json:"group,omitempty"`      // skill group name (set_skill_group)
	Items      []string `json:"items,omitempty"`      // skill group items (set_skill_group)
}

// Apply returns a copy of doc with the patch applied, or ErrInvalidPatch (leaving doc
// untouched) when the patch addresses something that does not exist. It never mutates its
// input: every edit builds fresh slices along the touched path, so the caller's document is
// safe to keep. The result is not sanitized — the caller runs Document.Sanitize before
// persisting.
func Apply(doc Document, p Patch) (Document, error) {
	switch p.Op {
	case PatchSetSummary:
		doc.Summary = p.Value
		return doc, nil
	case PatchSetHeaderField:
		return applyHeaderField(doc, p)
	case PatchAddBullet, PatchReplaceBullet, PatchRemoveBullet, PatchReorderBullets:
		return applyBulletOp(doc, p)
	case PatchSetSkillGroup:
		return applySkillGroup(doc, p)
	default:
		return doc, fmt.Errorf("%w: unknown op %q", ErrInvalidPatch, p.Op)
	}
}

func applyHeaderField(doc Document, p Patch) (Document, error) {
	switch p.Field {
	case "full_name":
		doc.Header.FullName = p.Value
	case "email":
		doc.Header.Email = p.Value
	case "phone":
		doc.Header.Phone = p.Value
	case "location":
		doc.Header.Location = p.Value
	default:
		return doc, fmt.Errorf("%w: unknown header field %q", ErrInvalidPatch, p.Field)
	}
	return doc, nil
}

func applyBulletOp(doc Document, p Patch) (Document, error) {
	if p.Experience < 0 || p.Experience >= len(doc.Experience) {
		return doc, fmt.Errorf("%w: experience index %d out of range", ErrInvalidPatch, p.Experience)
	}
	entry := doc.Experience[p.Experience]
	next, err := editBullets(entry.Bullets, p)
	if err != nil {
		return doc, err
	}
	entry.Bullets = next

	out := append([]ExperienceItem(nil), doc.Experience...)
	out[p.Experience] = entry
	doc.Experience = out
	return doc, nil
}

func editBullets(bullets []string, p Patch) ([]string, error) {
	switch p.Op {
	case PatchAddBullet:
		return append(append([]string(nil), bullets...), p.Value), nil
	case PatchReplaceBullet:
		if err := checkBulletIndex(bullets, p.Bullet); err != nil {
			return nil, err
		}
		next := append([]string(nil), bullets...)
		next[p.Bullet] = p.Value
		return next, nil
	case PatchRemoveBullet:
		if err := checkBulletIndex(bullets, p.Bullet); err != nil {
			return nil, err
		}
		next := append([]string(nil), bullets[:p.Bullet]...)
		return append(next, bullets[p.Bullet+1:]...), nil
	case PatchReorderBullets:
		return permute(bullets, p.Order)
	default:
		return nil, fmt.Errorf("%w: unknown op %q", ErrInvalidPatch, p.Op)
	}
}

func checkBulletIndex(bullets []string, i int) error {
	if i < 0 || i >= len(bullets) {
		return fmt.Errorf("%w: bullet index %d out of range", ErrInvalidPatch, i)
	}
	return nil
}

// permute returns bullets reordered so result[i] = bullets[order[i]]. order must be a
// permutation of the bullet indices — same length, every index in range, no repeats.
func permute(bullets []string, order []int) ([]string, error) {
	if len(order) != len(bullets) {
		return nil, fmt.Errorf("%w: order length %d does not match %d bullets", ErrInvalidPatch, len(order), len(bullets))
	}
	seen := make([]bool, len(bullets))
	out := make([]string, len(bullets))
	for dst, src := range order {
		if src < 0 || src >= len(bullets) || seen[src] {
			return nil, fmt.Errorf("%w: order is not a permutation of the bullets", ErrInvalidPatch)
		}
		seen[src] = true
		out[dst] = bullets[src]
	}
	return out, nil
}

func applySkillGroup(doc Document, p Patch) (Document, error) {
	groups := append([]SkillGroup(nil), doc.Skills...)
	items := append([]string(nil), p.Items...)
	for i := range groups {
		if groups[i].Group == p.Group {
			groups[i].Items = items
			doc.Skills = groups
			return doc, nil
		}
	}
	doc.Skills = append(groups, SkillGroup{Group: p.Group, Items: items})
	return doc, nil
}
