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
	"time"
)

func (p *Provider) send(ctx context.Context, sessionID string, payload []byte, responsesLite bool) (*http.Response, error) {
	credential, err := p.credentials.current(ctx)
	if err != nil {
		return nil, err
	}

	authRecovered := false
	for attempt := 0; attempt < 3; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/responses", bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("codex: create request: %w", err)
		}
		request.Header.Set("Authorization", "Bearer "+credential.accessToken)
		request.Header.Set("ChatGPT-Account-ID", credential.accountID)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "text/event-stream")
		request.Header.Set("User-Agent", p.userAgent)
		request.Header.Set("Version", p.protocolVersion)
		request.Header.Set("originator", defaultOriginator)
		request.Header.Set("session-id", sessionID)
		request.Header.Set("thread-id", sessionID)
		request.Header.Set("x-client-request-id", sessionID)
		request.Header.Set("x-codex-installation-id", sessionID)
		request.Header.Set("x-codex-window-id", sessionID)
		if responsesLite {
			request.Header.Set("x-openai-internal-codex-responses-lite", "true")
		}
		if credential.fedramp {
			request.Header.Set("X-OpenAI-Fedramp", "true")
		}

		response, requestErr := p.client.Do(request)
		if requestErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt < 2 {
				if err := waitForRetry(ctx, attempt); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("codex: send request: %w", requestErr)
		}

		if response.StatusCode == http.StatusUnauthorized && !authRecovered {
			_ = response.Body.Close()
			credential, err = p.credentials.recoverUnauthorized(ctx, credential.accessToken)
			if err != nil {
				return nil, err
			}
			authRecovered = true
			attempt--
			continue
		}
		if response.StatusCode >= 500 && attempt < 2 {
			_ = response.Body.Close()
			if err := waitForRetry(ctx, attempt); err != nil {
				return nil, err
			}
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			defer response.Body.Close()
			return nil, decodeHTTPError(response)
		}
		return response, nil
	}
	return nil, errors.New("codex: request retries exhausted")
}

func waitForRetry(ctx context.Context, attempt int) error {
	delay := initialRetryBackoff * time.Duration(1<<attempt)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func decodeHTTPError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maximumErrorBody))
	apiError := &APIError{StatusCode: response.StatusCode, Message: strings.TrimSpace(string(body))}
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		apiError.Code = envelope.Error.Code
		if apiError.Code == "" {
			apiError.Code = envelope.Error.Type
		}
		if envelope.Error.Message != "" {
			apiError.Message = envelope.Error.Message
		}
	}
	if apiError.Message == "" {
		apiError.Message = http.StatusText(response.StatusCode)
	}
	return apiError
}
