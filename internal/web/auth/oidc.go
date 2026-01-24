package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/foxzi/sendry/internal/web/config"
	"golang.org/x/oauth2"
)

// OIDCProvider handles OIDC authentication
type OIDCProvider struct {
	config   *config.OIDCConfig
	provider *oidc.Provider
	oauth2   oauth2.Config
	verifier *oidc.IDTokenVerifier

	mu     sync.RWMutex
	states map[string]struct{} // store valid state values
}

// NewOIDCProvider creates a new OIDC provider
func NewOIDCProvider(ctx context.Context, cfg *config.OIDCConfig) (*OIDCProvider, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
	}

	oauth2Config := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       cfg.Scopes,
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID})

	return &OIDCProvider{
		config:   cfg,
		provider: provider,
		oauth2:   oauth2Config,
		verifier: verifier,
		states:   make(map[string]struct{}),
	}, nil
}

// AuthCodeURL generates the authorization URL with a random state
func (p *OIDCProvider) AuthCodeURL() (string, string, error) {
	state, err := generateState()
	if err != nil {
		return "", "", err
	}

	p.mu.Lock()
	p.states[state] = struct{}{}
	p.mu.Unlock()

	url := p.oauth2.AuthCodeURL(state)
	return url, state, nil
}

// Exchange exchanges the authorization code for tokens and user info
func (p *OIDCProvider) Exchange(ctx context.Context, state, code string) (*UserInfo, error) {
	// Verify state
	p.mu.Lock()
	_, valid := p.states[state]
	if valid {
		delete(p.states, state)
	}
	p.mu.Unlock()

	if !valid {
		return nil, fmt.Errorf("invalid state")
	}

	// Exchange code for token
	token, err := p.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Get ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token in response")
	}

	// Verify ID token
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify id_token: %w", err)
	}

	// Extract claims
	var claims struct {
		Email         string   `json:"email"`
		EmailVerified bool     `json:"email_verified"`
		Name          string   `json:"name"`
		Groups        []string `json:"groups"`
	}

	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// Check allowed groups
	if len(p.config.AllowedGroups) > 0 {
		allowed := false
		for _, allowedGroup := range p.config.AllowedGroups {
			for _, userGroup := range claims.Groups {
				if userGroup == allowedGroup {
					allowed = true
					break
				}
			}
			if allowed {
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("user not in allowed groups")
		}
	}

	return &UserInfo{
		Email:  claims.Email,
		Name:   claims.Name,
		Groups: claims.Groups,
	}, nil
}

// UserInfo represents user information from OIDC
type UserInfo struct {
	Email  string
	Name   string
	Groups []string
}

func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
