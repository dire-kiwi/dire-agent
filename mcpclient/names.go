package mcpclient

import (
	"errors"
	"fmt"
)

const modelPrefix = "mcp__"

// ModelName returns the stable name presented to the model.
func ModelName(server, tool string) (string, error) {
	if err := validateName(server); err != nil {
		return "", fmt.Errorf("server name: %w", err)
	}
	if err := validateName(tool); err != nil {
		return "", fmt.Errorf("tool name: %w", err)
	}
	return modelPrefix + server + "__" + tool, nil
}

func validateName(name string) error {
	if name == "" {
		return errors.New("cannot be empty")
	}
	if len(name) > 128 {
		return errors.New("cannot exceed 128 bytes")
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.') {
			return fmt.Errorf("contains unsupported character %q", r)
		}
	}
	return nil
}
