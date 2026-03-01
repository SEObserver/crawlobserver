package gsc

import (
	"strings"
	"testing"

	"github.com/SEObserver/crawlobserver/internal/config"
)

func TestOAuthConfig(t *testing.T) {
	cfg := &config.GSCConfig{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
		RedirectURI:  "http://localhost:8080/callback",
	}

	oauthCfg := OAuthConfig(cfg)

	if oauthCfg.ClientID != "test-client-id" {
		t.Errorf("ClientID = %q, want %q", oauthCfg.ClientID, "test-client-id")
	}
	if oauthCfg.ClientSecret != "test-secret" {
		t.Errorf("ClientSecret = %q, want %q", oauthCfg.ClientSecret, "test-secret")
	}
	if oauthCfg.RedirectURL != "http://localhost:8080/callback" {
		t.Errorf("RedirectURL = %q, want %q", oauthCfg.RedirectURL, "http://localhost:8080/callback")
	}
	if len(oauthCfg.Scopes) != 1 || !strings.Contains(oauthCfg.Scopes[0], "webmasters") {
		t.Errorf("unexpected scopes: %v", oauthCfg.Scopes)
	}
}

func TestAuthorizeURL(t *testing.T) {
	cfg := &config.GSCConfig{
		ClientID:     "my-client",
		ClientSecret: "my-secret",
		RedirectURI:  "http://localhost/cb",
	}

	url := AuthorizeURL(cfg, "test-state")

	if !strings.Contains(url, "accounts.google.com") {
		t.Errorf("expected Google auth URL, got %q", url)
	}
	if !strings.Contains(url, "client_id=my-client") {
		t.Errorf("expected client_id in URL, got %q", url)
	}
	if !strings.Contains(url, "state=test-state") {
		t.Errorf("expected state in URL, got %q", url)
	}
	if !strings.Contains(url, "access_type=offline") {
		t.Errorf("expected access_type=offline in URL, got %q", url)
	}
	if !strings.Contains(url, "prompt=consent") {
		t.Errorf("expected prompt=consent in URL, got %q", url)
	}
}

func TestOAuthConfig_EmptyFields(t *testing.T) {
	cfg := &config.GSCConfig{}
	oauthCfg := OAuthConfig(cfg)

	if oauthCfg.ClientID != "" {
		t.Errorf("expected empty ClientID, got %q", oauthCfg.ClientID)
	}
}
