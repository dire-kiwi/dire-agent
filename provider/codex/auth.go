package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type credentialStore struct {
	path          string
	refreshURL    string
	oauthClient   string
	client        *http.Client
	now           func() time.Time
	refreshWindow time.Duration
	mu            sync.Mutex
}

type credential struct {
	accessToken string
	accountID   string
	plan        string
	fedramp     bool
}

type authDocument struct {
	raw      map[string]json.RawMessage
	authMode string
	tokens   tokenDocument
}

type tokenDocument struct {
	raw          map[string]json.RawMessage
	idToken      string
	accessToken  string
	refreshToken string
	accountID    string
}

type tokenClaims struct {
	ExpiresAt int64 `json:"exp"`
	Auth      struct {
		Plan      string `json:"chatgpt_plan_type"`
		AccountID string `json:"chatgpt_account_id"`
		FedRAMP   bool   `json:"chatgpt_account_is_fedramp"`
	} `json:"https://api.openai.com/auth"`
}

func (s *credentialStore) current(ctx context.Context) (credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	document, err := readAuthDocument(s.path)
	if err != nil {
		return credential{}, err
	}
	current, err := document.credential()
	if err != nil {
		return credential{}, err
	}
	if expiresSoon(document.tokens.accessToken, s.now(), s.refreshWindow) {
		return s.refreshLocked(ctx, document)
	}
	return current, nil
}

func (s *credentialStore) recoverUnauthorized(ctx context.Context, failedAccessToken string) (credential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	document, err := readAuthDocument(s.path)
	if err != nil {
		return credential{}, err
	}
	if document.tokens.accessToken != failedAccessToken {
		return document.credential()
	}
	return s.refreshLocked(ctx, document)
}

func (s *credentialStore) refreshLocked(ctx context.Context, document *authDocument) (credential, error) {
	if strings.TrimSpace(document.tokens.refreshToken) == "" {
		return credential{}, fmt.Errorf("%w: refresh token is missing", ErrNotAuthenticated)
	}

	requestBody, err := json.Marshal(map[string]string{
		"client_id":     s.oauthClient,
		"grant_type":    "refresh_token",
		"refresh_token": document.tokens.refreshToken,
	})
	if err != nil {
		return credential{}, fmt.Errorf("codex: encode token refresh: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.refreshURL, bytes.NewReader(requestBody))
	if err != nil {
		return credential{}, fmt.Errorf("codex: create token refresh: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", defaultUserAgent)

	response, err := s.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return credential{}, ctx.Err()
		}
		return credential{}, fmt.Errorf("codex: refresh access token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return credential{}, decodeHTTPError(response)
	}

	var refreshed struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maximumErrorBody)).Decode(&refreshed); err != nil {
		return credential{}, fmt.Errorf("codex: decode token refresh: %w", err)
	}
	if refreshed.AccessToken == "" {
		return credential{}, errors.New("codex: token refresh returned no access token")
	}

	// Avoid overwriting a newer login or refresh produced by another process.
	latest, err := readAuthDocument(s.path)
	if err != nil {
		return credential{}, err
	}
	if latest.tokens.accessToken != document.tokens.accessToken || latest.tokens.refreshToken != document.tokens.refreshToken {
		return latest.credential()
	}

	document.tokens.accessToken = refreshed.AccessToken
	if refreshed.IDToken != "" {
		document.tokens.idToken = refreshed.IDToken
	}
	if refreshed.RefreshToken != "" {
		document.tokens.refreshToken = refreshed.RefreshToken
	}
	if err := writeAuthDocument(s.path, document, s.now()); err != nil {
		return credential{}, err
	}
	return document.credential()
}
