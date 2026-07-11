package skills

import (
	"sort"
	"strings"
)

// InvocationSyntax identifies the explicit syntax used by the user.
type InvocationSyntax string

const (
	SyntaxDollar  InvocationSyntax = "dollar"
	SyntaxCommand InvocationSyntax = "command"
)

// Invocation is an explicit skill request found in user text. Start and End
// are byte offsets into the original string.
type Invocation struct {
	Name   string           `json:"name"`
	Args   string           `json:"args,omitempty"`
	Syntax InvocationSyntax `json:"syntax"`
	Start  int              `json:"start"`
	End    int              `json:"end"`
}

// DetectInvocations finds $skill-name references anywhere and
// /skill:name args commands at the beginning of a line.
func DetectInvocations(text string) []Invocation {
	invocations := detectDollarInvocations(text)
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		withoutNewline := strings.TrimSuffix(line, "\n")
		leading := len(withoutNewline) - len(strings.TrimLeft(withoutNewline, " \t"))
		command := withoutNewline[leading:]
		if strings.HasPrefix(command, "/skill:") {
			nameStart := len("/skill:")
			nameEnd := nameStart
			for nameEnd < len(command) && isNameByte(command[nameEnd]) {
				nameEnd++
			}
			if nameEnd > nameStart && (nameEnd == len(command) || command[nameEnd] == ' ' || command[nameEnd] == '\t') {
				invocations = append(invocations, Invocation{
					Name: command[nameStart:nameEnd], Args: strings.TrimSpace(command[nameEnd:]),
					Syntax: SyntaxCommand, Start: offset + leading, End: offset + len(withoutNewline),
				})
			}
		}
		offset += len(line)
	}
	sort.SliceStable(invocations, func(i, j int) bool { return invocations[i].Start < invocations[j].Start })
	return invocations
}

func detectDollarInvocations(text string) []Invocation {
	var result []Invocation
	for index := 0; index < len(text); index++ {
		if text[index] != '$' || (index > 0 && text[index-1] == '\\') {
			continue
		}
		end := index + 1
		for end < len(text) && isNameByte(text[end]) {
			end++
		}
		if end == index+1 || end-index-1 > 64 {
			continue
		}
		name := text[index+1 : end]
		if compatibleName.MatchString(name) && !strings.Contains(name, "--") {
			result = append(result, Invocation{Name: name, Syntax: SyntaxDollar, Start: index, End: end})
			index = end - 1
		}
	}
	return result
}

func isNameByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '-' || value == '_'
}

// ResolveInvocations returns known enabled requests plus diagnostics for
// unknown or disabled names.
func (c *Catalog) ResolveInvocations(text string) ([]Invocation, []Diagnostic) {
	var resolved []Invocation
	var issues []Diagnostic
	for _, invocation := range DetectInvocations(text) {
		skill, found := c.FindAny(invocation.Name)
		if !found {
			issues = append(issues, Diagnostic{
				Severity: SeverityWarning, Code: "unknown-invocation",
				Message: "unknown skill requested: " + invocation.Name,
			})
			continue
		}
		if !skill.Enabled {
			issues = append(issues, Diagnostic{
				Severity: SeverityWarning, Code: "disabled-invocation", Path: skill.Path,
				Message: "disabled skill requested: " + skill.Name,
			})
			continue
		}
		invocation.Name = skill.Name
		resolved = append(resolved, invocation)
	}
	return resolved, issues
}
