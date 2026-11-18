package tracerlink

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"
)

// Repository is the owner-scoped storage this service needs, so a caller can stand it in
// without a database.
type Repository interface {
	// Upsert returns the token already standing for this (CV, position, destination), minting
	// the offered one if there is none. It writes nothing for a CV the user does not own.
	Upsert(ctx context.Context, cvID uuid.UUID, userID int64, token, sourcePath, destURL, destHash string) (string, error)
}

// Minter issues the tokens a traced render substitutes in.
type Minter struct {
	repo     Repository
	ownHosts []string
}

func NewMinter(repo Repository, ownHosts []string) *Minter {
	return &Minter{repo: repo, ownHosts: ownHosts}
}

// Hrefs holds one link target per position, aligned by index with the CV's own link slices. It
// mirrors cv.LinkHrefs without depending on it: this package knows about links, not about
// rendering.
type Hrefs struct {
	Header   []string
	Projects []string
}

// Mint issues a token for every traceable link of a CV and returns where each one should point.
//
// It runs on every download (the PDF is never stored), so the upsert must be idempotent: an
// unchanged CV yields the same tokens.
//
// A link that fails to mint is left out rather than failing the render.
func (m *Minter) Mint(ctx context.Context, cvID uuid.UUID, userID int64, baseURL, prefix string,
	headerLinks, projectLinks []string) Hrefs {
	out := Hrefs{
		Header:   make([]string, len(headerLinks)),
		Projects: make([]string, len(projectLinks)),
	}
	for _, t := range Targets(m.ownHosts, headerLinks, projectLinks) {
		sum := sha256.Sum256([]byte(t.URL))
		token, err := m.repo.Upsert(ctx, cvID, userID, Token(prefix), t.SourcePath(), t.URL, hex.EncodeToString(sum[:]))
		if err != nil || token == "" {
			continue
		}
		traced := baseURL + "/cv/" + token
		switch t.Section {
		case SectionHeaderLinks:
			out.Header[t.Index] = traced
		case SectionProjectLink:
			out.Projects[t.Index] = traced
		}
	}
	return out
}

// QueriesRepository adapts the generated sqlc queries to Repository.
type QueriesRepository struct {
	upsert func(ctx context.Context, cvID uuid.UUID, userID int64, token, sourcePath, destURL, destHash string) (string, error)
}

// NewRepository wraps the caller's upsert. Taking a function rather than *db.Queries keeps
// this package free of the database import.
func NewRepository(upsert func(ctx context.Context, cvID uuid.UUID, userID int64, token, sourcePath, destURL, destHash string) (string, error)) Repository {
	return QueriesRepository{upsert: upsert}
}

func (r QueriesRepository) Upsert(ctx context.Context, cvID uuid.UUID, userID int64, token, sourcePath, destURL, destHash string) (string, error) {
	return r.upsert(ctx, cvID, userID, token, sourcePath, destURL, destHash)
}
