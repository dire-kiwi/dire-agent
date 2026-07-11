package skills

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var compatibleName = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9_-]*[A-Za-z0-9])?$`)

// ParseFrontmatter extracts name and description without requiring a YAML
// dependency. It supports quoted scalars and YAML literal/folded block values.
func ParseFrontmatter(contents []byte, path string) (Metadata, []Diagnostic) {
	var metadata Metadata
	if !utf8.Valid(contents) {
		return metadata, []Diagnostic{{
			Severity: SeverityError, Code: "invalid-utf8", Path: path,
			Message: "SKILL.md must contain valid UTF-8",
		}}
	}
	text := strings.TrimPrefix(string(contents), "\ufeff")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return metadata, []Diagnostic{{
			Severity: SeverityError, Code: "missing-frontmatter", Path: path, Line: 1,
			Message: "SKILL.md must start with YAML frontmatter delimited by ---",
		}}
	}
	end := -1
	for index := 1; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			end = index
			break
		}
	}
	if end < 0 {
		return metadata, []Diagnostic{{
			Severity: SeverityError, Code: "unterminated-frontmatter", Path: path, Line: 1,
			Message: "frontmatter is missing its closing --- delimiter",
		}}
	}

	values := make(map[string]string)
	valueLines := make(map[string]int)
	var issues []Diagnostic
	for index := 1; index < end; index++ {
		line := lines[index]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || leadingSpaces(line) > 0 {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon < 1 {
			issues = append(issues, Diagnostic{
				Severity: SeverityWarning, Code: "malformed-field", Path: path, Line: index + 1,
				Message: "ignoring frontmatter line without a key/value separator",
			})
			continue
		}
		key := strings.TrimSpace(line[:colon])
		if key != "name" && key != "description" {
			continue
		}
		if _, exists := values[key]; exists {
			issues = append(issues, Diagnostic{
				Severity: SeverityError, Code: "duplicate-field", Path: path, Line: index + 1,
				Message: fmt.Sprintf("frontmatter field %q is specified more than once", key),
			})
			continue
		}
		lineNumber := index + 1
		raw := strings.TrimSpace(line[colon+1:])
		var value string
		if raw == "|" || raw == ">" || raw == "|-" || raw == ">-" {
			block, next := readBlock(lines, index+1, end, leadingSpaces(line))
			index = next - 1
			if strings.HasPrefix(raw, ">") {
				value = foldBlock(block)
			} else {
				value = strings.Join(block, "\n")
			}
		} else {
			parsed, err := parseScalar(raw)
			if err != nil {
				issues = append(issues, Diagnostic{
					Severity: SeverityError, Code: "invalid-scalar", Path: path, Line: index + 1,
					Message: fmt.Sprintf("invalid %s value: %v", key, err),
				})
				continue
			}
			value = parsed
		}
		values[key], valueLines[key] = strings.TrimSpace(value), lineNumber
	}

	metadata = Metadata{Name: values["name"], Description: values["description"]}
	issues = append(issues, validateMetadata(metadata, path, valueLines)...)
	return metadata, issues
}

func parseScalar(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if raw[0] == '"' {
		value, err := strconv.Unquote(raw)
		if err != nil {
			return "", err
		}
		return value, nil
	}
	if raw[0] == '\'' {
		if len(raw) < 2 || raw[len(raw)-1] != '\'' {
			return "", fmt.Errorf("unterminated single-quoted string")
		}
		return strings.ReplaceAll(raw[1:len(raw)-1], "''", "'"), nil
	}
	if comment := strings.Index(raw, " #"); comment >= 0 {
		raw = raw[:comment]
	}
	return strings.TrimSpace(raw), nil
}

func readBlock(lines []string, start, end, parentIndent int) ([]string, int) {
	next := start
	minimum := -1
	for next < end {
		line := lines[next]
		if strings.TrimSpace(line) != "" && leadingSpaces(line) <= parentIndent {
			break
		}
		if strings.TrimSpace(line) != "" {
			indent := leadingSpaces(line)
			if minimum < 0 || indent < minimum {
				minimum = indent
			}
		}
		next++
	}
	if minimum < 0 {
		minimum = parentIndent + 1
	}
	result := make([]string, 0, next-start)
	for _, line := range lines[start:next] {
		if strings.TrimSpace(line) == "" {
			result = append(result, "")
			continue
		}
		if len(line) >= minimum {
			line = line[minimum:]
		}
		result = append(result, strings.TrimRight(line, " \t"))
	}
	return result, next
}

func foldBlock(lines []string) string {
	var result strings.Builder
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			blank = true
			continue
		}
		if result.Len() > 0 {
			if blank {
				result.WriteByte('\n')
			} else {
				result.WriteByte(' ')
			}
		}
		result.WriteString(line)
		blank = false
	}
	return result.String()
}

func validateMetadata(metadata Metadata, path string, lines map[string]int) []Diagnostic {
	var issues []Diagnostic
	if metadata.Name == "" {
		issues = append(issues, Diagnostic{Severity: SeverityError, Code: "missing-name", Path: path, Message: "frontmatter name is required"})
	} else if utf8.RuneCountInString(metadata.Name) > 64 {
		issues = append(issues, Diagnostic{Severity: SeverityError, Code: "name-too-long", Path: path, Line: lines["name"], Message: "skill name must be at most 64 characters"})
	} else if !compatibleName.MatchString(metadata.Name) || strings.Contains(metadata.Name, "--") {
		issues = append(issues, Diagnostic{Severity: SeverityError, Code: "invalid-name", Path: path, Line: lines["name"], Message: "skill name may contain only letters, numbers, hyphens, and underscores"})
	} else if metadata.Name != strings.ToLower(metadata.Name) || strings.Contains(metadata.Name, "_") {
		issues = append(issues, Diagnostic{Severity: SeverityWarning, Code: "nonstandard-name", Path: path, Line: lines["name"], Message: "Agent Skills names should use lowercase letters, numbers, and hyphens"})
	}
	if metadata.Description == "" {
		issues = append(issues, Diagnostic{Severity: SeverityError, Code: "missing-description", Path: path, Message: "frontmatter description is required"})
	} else if utf8.RuneCountInString(metadata.Description) > 1024 {
		issues = append(issues, Diagnostic{Severity: SeverityError, Code: "description-too-long", Path: path, Line: lines["description"], Message: "skill description must be at most 1024 characters"})
	}
	return issues
}

func leadingSpaces(value string) int {
	count := 0
	for count < len(value) && value[count] == ' ' {
		count++
	}
	return count
}
