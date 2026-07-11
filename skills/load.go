package skills

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// Load returns the complete SKILL.md for an enabled catalog entry.
func (c *Catalog) Load(name string) (string, error) {
	skill, ok := c.FindAny(name)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrSkillNotFound, strings.TrimSpace(name))
	}
	if !skill.Enabled {
		return "", fmt.Errorf("%w: %s (%s)", ErrSkillDisabled, skill.Name, skill.DisabledReason)
	}
	current := canonical(skill.Path)
	if current != skill.Path {
		return "", fmt.Errorf("skill %q path changed; rediscover skills before loading", skill.Name)
	}
	maximum := c.maxBytes
	if maximum <= 0 {
		maximum = DefaultMaxSkillBytes
	}
	contents, issue := readBounded(skill.Path, maximum)
	if issue != nil {
		return "", fmt.Errorf("load skill %q: %s", skill.Name, issue.Message)
	}
	metadata, issues := ParseFrontmatter(contents, skill.Path)
	if hasErrors(issues) || !strings.EqualFold(metadata.Name, skill.Name) {
		return "", fmt.Errorf("skill %q changed since discovery; rediscover skills before loading", skill.Name)
	}
	return string(contents), nil
}

func readBounded(path string, maximum int64) ([]byte, *Diagnostic) {
	file, err := os.Open(path)
	if err != nil {
		return nil, &Diagnostic{Severity: SeverityError, Code: "file-unreadable", Path: path, Message: err.Error()}
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, &Diagnostic{Severity: SeverityError, Code: "file-unreadable", Path: path, Message: err.Error()}
	}
	if !info.Mode().IsRegular() {
		return nil, &Diagnostic{Severity: SeverityError, Code: "not-regular-file", Path: path, Message: "SKILL.md is not a regular file"}
	}
	if info.Size() > maximum {
		return nil, &Diagnostic{Severity: SeverityError, Code: "file-too-large", Path: path, Message: fmt.Sprintf("SKILL.md exceeds %d bytes", maximum)}
	}
	contents, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, &Diagnostic{Severity: SeverityError, Code: "file-unreadable", Path: path, Message: err.Error()}
	}
	if int64(len(contents)) > maximum {
		return nil, &Diagnostic{Severity: SeverityError, Code: "file-too-large", Path: path, Message: fmt.Sprintf("SKILL.md exceeds %d bytes", maximum)}
	}
	if !utf8.Valid(contents) || bytes.IndexByte(contents, 0) >= 0 {
		return nil, &Diagnostic{Severity: SeverityError, Code: "invalid-text", Path: path, Message: "SKILL.md must be UTF-8 text without NUL bytes"}
	}
	return contents, nil
}
