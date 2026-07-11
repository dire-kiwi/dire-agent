package extensions

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const maxModelToolName = 64

// ModelName returns the stable model-facing namespace for an extension tool.
func ModelName(extensionID, toolName string) string {
	id := normalizeSegment(extensionID, "extension", true)
	tool := normalizeSegment(toolName, "tool", false)
	name := "ext__" + id + "__" + tool
	if len(name) <= maxModelToolName {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	suffix := "__" + hex.EncodeToString(sum[:4])
	return name[:maxModelToolName-len(suffix)] + suffix
}

func normalizeID(value string) string {
	return normalizeSegment(value, "extension", true)
}

func normalizeSegment(value, fallback string, lower bool) string {
	value = strings.TrimSpace(value)
	var output strings.Builder
	underscore := false
	for _, char := range value {
		valid := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' ||
			char >= '0' && char <= '9' || char == '-' || char == '_'
		if !valid {
			if output.Len() > 0 {
				underscore = true
			}
			continue
		}
		if underscore && output.Len() > 0 {
			output.WriteByte('_')
			underscore = false
		}
		if lower && char >= 'A' && char <= 'Z' {
			char += 'a' - 'A'
		}
		output.WriteRune(char)
	}
	result := strings.Trim(output.String(), "_-")
	if result == "" {
		return fallback
	}
	return result
}
