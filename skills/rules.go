package skills

import (
	"path/filepath"
	"strings"
)

type compiledRule struct {
	path    string
	enabled bool
	order   int
}

type ruleSet []compiledRule

func compileRules(config Config) ruleSet {
	var rules ruleSet
	add := func(paths []string, enabled bool) {
		for _, path := range paths {
			if normalized := canonical(path); normalized != "" {
				rules = append(rules, compiledRule{path: normalized, enabled: enabled, order: len(rules)})
			}
		}
	}
	// A more-specific enabled child can opt back into a disabled subtree.
	add(config.EnabledPaths, true)
	add(config.DisabledPaths, false)
	for _, rule := range config.PathRules {
		if path := canonical(rule.Path); path != "" {
			rules = append(rules, compiledRule{path: path, enabled: rule.Enabled, order: len(rules)})
		}
	}
	return rules
}

func (rules ruleSet) enabled(path, directory string) (bool, string) {
	var winner *compiledRule
	for index := range rules {
		rule := &rules[index]
		if !containsPath(rule.path, path) && !containsPath(rule.path, directory) {
			continue
		}
		if winner == nil || len(rule.path) > len(winner.path) ||
			(len(rule.path) == len(winner.path) && rule.order > winner.order) {
			winner = rule
		}
	}
	if winner == nil || winner.enabled {
		return true, ""
	}
	return false, "disabled by path rule " + winner.path
}

func containsPath(parent, child string) bool {
	if samePath(parent, child) {
		return true
	}
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func samePath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
