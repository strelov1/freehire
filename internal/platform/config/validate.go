package config

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

// Validate checks the configuration is safe to boot the auth surface with. The JWT secret
// check runs in every environment: a short HS256 secret is brute-forceable offline from any
// captured token.
func (s Settings) Validate() error {
	if s.Env == "production" && !s.CookieSecure {
		return errors.New("COOKIE_SECURE must be true in production")
	}
	if strings.HasPrefix(s.FrontendOrigin, "https://") && !s.CookieSecure {
		return errors.New("COOKIE_SECURE must be true when using HTTPS origin")
	}
	if s.AuthIntrospectionSecret != "" && len(s.AuthIntrospectionSecret) < 32 {
		return errors.New("AUTH_INTROSPECTION_SECRET must be at least 32 bytes")
	}
	if len(s.JWTSecret) < 32 {
		return errors.New("JWT_SECRET is required and must be at least 32 bytes")
	}
	if s.RecentAuthTTL != 0 && (s.RecentAuthTTL < time.Minute || s.RecentAuthTTL > time.Hour) {
		return errors.New("RECENT_AUTH_TTL must be between 1m and 1h")
	}
	appleWeb := s.OAuth["apple"]
	appleFields := []bool{s.AppleNativeClientID != "", s.AppleGrantActiveKeyID != "", len(s.AppleGrantKeys) != 0}
	partialApple := (appleFields[0] || appleFields[1] || appleFields[2]) && (!appleFields[0] || !appleFields[1] || !appleFields[2])
	if partialApple {
		return errors.New("native Apple requires APPLE_NATIVE_CLIENT_ID, APPLE_GRANT_ACTIVE_KEY_ID, and APPLE_GRANT_KEYS")
	}
	if appleFields[0] {
		if appleWeb.TeamID == "" || appleWeb.KeyID == "" || appleWeb.PrivateKey == "" {
			return errors.New("native Apple requires OAUTH_APPLE_TEAM_ID, OAUTH_APPLE_KEY_ID, and OAUTH_APPLE_PRIVATE_KEY")
		}
		if len(s.AppleGrantKeys[s.AppleGrantActiveKeyID]) != 32 {
			return errors.New("APPLE_GRANT_ACTIVE_KEY_ID must identify a 32-byte key in APPLE_GRANT_KEYS")
		}
	}
	if s.AuthV2Enabled && len(s.MobileAuthCallbacks) == 0 {
		return errors.New("MOBILE_AUTH_CALLBACKS is required when AUTH_V2_ENABLED=true")
	}
	if s.AuthV2Enabled && s.MobileAuthCallbacks["web"] == "" {
		return errors.New("MOBILE_AUTH_CALLBACKS must include web when AUTH_V2_ENABLED=true")
	}
	for name, raw := range s.MobileAuthCallbacks {
		u, err := url.Parse(raw)
		validName := name == "ios" || name == "android" || name == "web"
		if err != nil || !validName || !u.IsAbs() || u.Host == "" || u.User != nil || u.Fragment != "" {
			return errors.New("MOBILE_AUTH_CALLBACKS must contain valid ios, android, or web URLs")
		}
		if s.Env == "production" && u.Scheme != "https" {
			return errors.New("MOBILE_AUTH_CALLBACKS must use HTTPS in production")
		}
	}
	if raw := s.MobileAuthCallbacks["web"]; raw != "" {
		callback, callbackErr := url.Parse(raw)
		frontend, frontendErr := url.Parse(s.FrontendOrigin)
		if callbackErr != nil || frontendErr != nil || callback.Scheme != frontend.Scheme || callback.Host != frontend.Host || callback.Path != "/my/reauth" {
			return errors.New("MOBILE_AUTH_CALLBACKS web must be FRONTEND_ORIGIN/my/reauth")
		}
	}
	return nil
}
