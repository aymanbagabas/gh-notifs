package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

func (a *app) suggestions() ([]string, int, int) {
	if !a.edit {
		return nil, 0, 0
	}
	key := fmt.Sprintf("%d:%s", a.caret, string(a.input))
	if key != a.completionKey {
		a.completionKey, a.completionIndex = key, -1
	}
	start, end := 0, len(a.input)
	var quote rune
	for i, r := range a.input {
		if i == a.caret {
			break
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
		} else if r == '"' || r == '\'' {
			quote = r
		} else if unicode.IsSpace(r) {
			start = i + 1
		}
	}
	for i := a.caret; i < len(a.input); i++ {
		r := a.input[i]
		if quote != 0 {
			if r == quote {
				quote = 0
			}
		} else if r == '"' || r == '\'' {
			quote = r
		} else if unicode.IsSpace(r) {
			end = i
			break
		}
	}
	prefix := strings.ToLower(string(a.input[start:a.caret]))
	if prefix == "" && strings.TrimSpace(string(a.input)) != "" && a.completionIndex < 0 {
		return nil, start, end
	}
	if prefix != "" && a.completionIndex < 0 {
		a.completionIndex = 0
	}
	field, value, hasField := strings.Cut(prefix, ":")
	values := []string{"repo:", "org:", "reason:", "is:", "author:", "state:", "since:", "before:"}
	if hasField {
		value = strings.Trim(value, `"'`)
		switch field {
		case "is":
			values = []string{"read", "unread", "issue", "pr", "pull-request", "issue-or-pull-request",
				"discussion", "release", "commit", "check-suite", "repository-invitation",
				"repository-vulnerability-alert", "repository-advisory", "gist"}
		case "reason":
			values = []string{"assign", "author", "ci-activity", "comment", "invitation", "manual", "participating",
				"mention", "review-requested", "security-alert", "state-change", "subscribed", "team-mention"}
		case "state":
			values = []string{"open", "closed", "merged", "draft"}
		case "author", "repo", "org":
			values = nil
			if field == "author" {
				values = append(values, "@me")
			}
			for _, item := range a.result.Items {
				switch field {
				case "repo":
					values = append(values, item.Repository)
				case "org":
					owner, _, _ := strings.Cut(item.Repository, "/")
					values = append(values, owner)
				case "author":
					values = append(values, item.Author)
				}
			}
		case "since", "before":
			today := time.Now().UTC()
			values = []string{today.Format(time.DateOnly), today.AddDate(0, 0, -7).Format(time.DateOnly)}
		default:
			return nil, start, end
		}
	}
	seen := make(map[string]bool)
	var out []string
	if field == "repo" || field == "org" || field == "author" {
		sort.Strings(values)
	}
	for _, candidate := range values {
		if candidate == "" || strings.ContainsAny(candidate, " \t\n\r\"'") {
			continue
		}
		match := prefix
		if hasField {
			match = value
		}
		if !strings.HasPrefix(strings.ToLower(candidate), match) {
			continue
		}
		if hasField {
			candidate = field + ":" + candidate
		}
		if strings.EqualFold(candidate, string(a.input[start:end])) || seen[candidate] {
			continue
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	a.completionIndex = min(a.completionIndex, max(0, len(out)-1))
	return out, start, end
}

func (a *app) complete(value string, start, end int) {
	input := append([]rune(nil), a.input[:start]...)
	input = append(input, []rune(value)...)
	a.caret = len(input)
	a.input = append(input, a.input[end:]...)
	a.completionIndex = 0
}
