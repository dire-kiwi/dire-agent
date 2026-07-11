package skills

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// CatalogText returns the progressive-disclosure prompt fragment. It includes
// metadata only; full instructions remain unloaded until requested.
func (c *Catalog) CatalogText() string {
	return c.CatalogTextLimit(MaxCatalogChars)
}

// CatalogTextLimit applies a caller limit in addition to MaxCatalogChars.
func (c *Catalog) CatalogTextLimit(limit int) string {
	if limit <= 0 || limit > MaxCatalogChars {
		limit = MaxCatalogChars
	}
	header := "<available_skills>\nLoad a skill with the skill tool only when its instructions are needed.\n"
	footer := "</available_skills>"
	enabled := c.EnabledSkills()
	if len(enabled) == 0 {
		return truncateRunes(header+"(none)\n"+footer, limit)
	}
	lines := make([]string, 0, len(enabled))
	for _, skill := range enabled {
		description := strings.Join(strings.Fields(skill.Description), " ")
		lines = append(lines, fmt.Sprintf("- %s: %s\n", skill.Name, description))
	}
	full := header + strings.Join(lines, "") + footer
	if utf8.RuneCountInString(full) <= limit {
		return full
	}

	var body strings.Builder
	for index, line := range lines {
		remaining := len(lines) - index
		omitted := fmt.Sprintf("- … %d more skill(s); use the skill tool to list them.\n", remaining)
		candidate := header + body.String() + line + footer
		if utf8.RuneCountInString(candidate) <= limit {
			body.WriteString(line)
			continue
		}
		candidate = header + body.String() + omitted + footer
		if utf8.RuneCountInString(candidate) <= limit {
			return candidate
		}
		break
	}
	return truncateRunes(header+body.String()+footer, limit)
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
