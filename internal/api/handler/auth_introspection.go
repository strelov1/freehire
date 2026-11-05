package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/platform/db"
)

const authIntrospectionSecretHeader = "X-FH-Auth-Secret"

type authIntrospectionStore interface {
	auth.TokenVersionLoader
	ResolveAPIKeyIdentity(context.Context, string) (db.ResolveAPIKeyIdentityRow, error)
	ResolveOAuthGrantIdentity(context.Context, string) (db.ResolveOAuthGrantIdentityRow, error)
	GetUserAPIEntitlement(context.Context, int64) (db.GetUserAPIEntitlementRow, error)
}

type introspectedEntitlements struct {
	ProUntil     int64 `json:"pro_until"`
	UltraUntil   int64 `json:"ultra_until"`
	APIUntil     int64 `json:"api_until"`
	APIDataUntil int64 `json:"api_data_until"`
}

type introspectedIdentity struct {
	UserID         string                   `json:"user_id"`
	CredentialKind string                   `json:"credential_kind"`
	Scope          string                   `json:"scope"`
	Entitlements   introspectedEntitlements `json:"entitlements"`
	ExpiresAt      int64                    `json:"expires_at,omitempty"`
}

// newAuthIntrospection exposes verified credentials to trusted infrastructure, never client identity headers.
func newAuthIntrospection(store authIntrospectionStore, issuer *auth.Issuer, secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		if secret == "" {
			return fiber.ErrNotFound
		}
		if subtle.ConstantTimeCompare([]byte(c.Get(authIntrospectionSecretHeader)), []byte(secret)) != 1 {
			return fiber.ErrForbidden
		}
		ctx, cancel := context.WithTimeout(c.Context(), 150*time.Millisecond)
		defer cancel()
		if token := c.Cookies(auth.CookieName); token != "" {
			identity, err := introspectSession(ctx, store, issuer, token)
			if err == nil {
				return c.JSON(identity)
			}
			if !errors.Is(err, fiber.ErrUnauthorized) {
				return err
			}
		}

		token := auth.PresentedBearer(c)
		var identity introspectedIdentity
		switch {
		case auth.WellFormedAPIKey(token):
			row, err := store.ResolveAPIKeyIdentity(ctx, auth.HashAPIKey(token))
			if err != nil {
				return introspectionError(err)
			}
			identity = introspectedIdentity{
				UserID: strconv.FormatInt(row.UserID, 10), CredentialKind: "api_key", Scope: row.Scope,
				ExpiresAt:    entitlementTime(row.ExpiresAt),
				Entitlements: entitlementTimes(row.ProUntil, row.UltraUntil, row.ApiUntilStripe, row.ApiDataUntilStripe),
			}
		case oauth2server.WellFormedAccessToken(token):
			row, err := store.ResolveOAuthGrantIdentity(ctx, oauth2server.HashToken(token))
			if err != nil {
				return introspectionError(err)
			}
			identity = introspectedIdentity{
				UserID: strconv.FormatInt(row.UserID, 10), CredentialKind: "oauth_grant", Scope: auth.ScopeFull,
				ExpiresAt:    entitlementTime(row.ExpiresAt),
				Entitlements: entitlementTimes(row.ProUntil, row.UltraUntil, row.ApiUntilStripe, row.ApiDataUntilStripe),
			}
		case token != "":
			var err error
			identity, err = introspectSession(ctx, store, issuer, token)
			if err != nil {
				return err
			}
		default:
			return fiber.ErrUnauthorized
		}
		id, err := strconv.ParseInt(identity.UserID, 10, 64)
		if err != nil || id <= 0 {
			return fiber.ErrUnauthorized
		}
		return c.JSON(identity)
	}
}

func introspectSession(ctx context.Context, store authIntrospectionStore, issuer *auth.Issuer, token string) (introspectedIdentity, error) {
	var identity introspectedIdentity
	if issuer == nil {
		return identity, fiber.ErrUnauthorized
	}
	id, version, err := issuer.Parse(token)
	if err != nil || id <= 0 {
		return identity, fiber.ErrUnauthorized
	}
	current, err := store.GetUserTokenVersion(ctx, id)
	if err != nil {
		return identity, introspectionError(err)
	}
	if current != version {
		return identity, fiber.ErrUnauthorized
	}
	row, err := store.GetUserAPIEntitlement(ctx, id)
	if err != nil {
		return identity, introspectionError(err)
	}
	expiry, err := issuer.ExpiresAt(token)
	if err != nil {
		return identity, fiber.ErrUnauthorized
	}
	identity = introspectedIdentity{
		UserID: strconv.FormatInt(id, 10), CredentialKind: "session", Scope: auth.ScopeFull,
		Entitlements: entitlementTimes(row.ProUntil, row.UltraUntil, row.ApiUntilStripe, row.ApiDataUntilStripe),
	}
	if !expiry.IsZero() {
		identity.ExpiresAt = expiry.Unix()
	}
	return identity, nil
}

func introspectionError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.ErrUnauthorized
	}
	return fiber.ErrServiceUnavailable
}

func entitlementTime(value pgtype.Timestamptz) int64 {
	if !value.Valid || value.InfinityModifier != pgtype.Finite {
		return 0
	}
	return value.Time.Unix()
}

func entitlementTimes(pro, ultra, api, data pgtype.Timestamptz) introspectedEntitlements {
	return introspectedEntitlements{ProUntil: entitlementTime(pro), UltraUntil: entitlementTime(ultra), APIUntil: entitlementTime(api), APIDataUntil: entitlementTime(data)}
}
