package mcpclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	ErrClosed       = errors.New("MCP client is closed")
	ErrDisabled     = errors.New("MCP server is disabled")
	ErrUntrusted    = errors.New("MCP server is not trusted")
	ErrNotConnected = errors.New("MCP server is not connected")
	ErrUnknownTool  = errors.New("unknown MCP tool")
	ErrToolFailed   = errors.New("MCP tool reported an error")
)

// ConfigError identifies invalid configuration without echoing secret values.
type ConfigError struct {
	Server  string
	Message string
}

func (e *ConfigError) Error() string {
	if e.Server == "" {
		return "MCP configuration: " + e.Message
	}
	return fmt.Sprintf("MCP server %q configuration: %s", e.Server, e.Message)
}

func safeError(cfg ServerConfig, operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	message := err.Error()
	if cfg.Endpoint != "" {
		message = strings.ReplaceAll(message, cfg.Endpoint, safeEndpoint(cfg.Endpoint))
	}
	secrets := make([]string, 0, len(cfg.Environment)+len(cfg.Headers)+4)
	for _, value := range cfg.Environment {
		secrets = append(secrets, value)
	}
	for _, value := range cfg.Headers {
		secrets = append(secrets, value)
	}
	if parsed, parseErr := url.Parse(cfg.Endpoint); parseErr == nil {
		if parsed.User != nil {
			secrets = append(secrets, parsed.User.Username())
			if password, ok := parsed.User.Password(); ok {
				secrets = append(secrets, password)
			}
		}
		for _, values := range parsed.Query() {
			secrets = append(secrets, values...)
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	message, _ = truncateUTF8(message, 1024)
	return fmt.Errorf("MCP server %q %s: %s", cfg.Name, operation, message)
}

func safeEndpoint(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "[redacted endpoint]"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func truncateUTF8(value string, limit int) (string, bool) {
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	const marker = "…"
	if limit < len(marker) {
		return strings.Repeat(".", limit), true
	}
	value = value[:limit-len(marker)]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value + marker, true
}
