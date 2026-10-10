package notifs

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

type Term struct {
	Field string
	Value string
}

type Query struct {
	Raw   string
	Terms []Term
}

func ParseQuery(input string) (Query, error) {
	tokens, err := splitQuery(input)
	if err != nil {
		return Query{}, err
	}
	q := Query{Raw: input}
	for _, token := range tokens {
		field, value, found := strings.Cut(token, ":")
		if !found {
			field, value = "text", token
		}
		field = strings.ToLower(field)
		if value == "" {
			return Query{}, fmt.Errorf("query %q needs a value", field)
		}
		switch field {
		case "repo":
			if len(strings.Split(value, "/")) != 2 || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
				return Query{}, fmt.Errorf("repo must be OWNER/REPO")
			}
		case "org", "author", "text", "reason":
		case "is":
			value = TypeName(value)
			switch value {
			case "read", "unread", "issue", "pr", "pull-request", "issue-or-pull-request",
				"discussion", "release", "commit", "check-suite", "repository-invitation",
				"repository-vulnerability-alert", "repository-advisory", "gist":
			case "saved", "done":
				return Query{}, fmt.Errorf("is:%s is not available through the public notifications API", value)
			default:
				return Query{}, fmt.Errorf("unsupported is: filter %q", value)
			}
		case "state":
			switch strings.ToLower(value) {
			case "open", "closed", "merged", "draft":
			default:
				return Query{}, fmt.Errorf("unsupported state %q", value)
			}
		case "since", "before":
			if _, err := ParseDate(value); err != nil {
				return Query{}, err
			}
		default:
			return Query{}, fmt.Errorf("unsupported query field %q", field)
		}
		q.Terms = append(q.Terms, Term{Field: field, Value: value})
	}
	return q, nil
}

func ParseDate(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, time.DateOnly} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q: use YYYY-MM-DD or RFC3339", value)
}

func (q Query) NeedsDetails() bool {
	for _, t := range q.Terms {
		if t.Field == "author" || t.Field == "state" {
			return true
		}
	}
	return false
}

func (q Query) Match(t Thread, login string) bool {
	for _, term := range q.Terms {
		value := strings.ToLower(term.Value)
		switch term.Field {
		case "repo":
			if !strings.EqualFold(t.Repository, value) {
				return false
			}
		case "org":
			owner, _, _ := strings.Cut(t.Repository, "/")
			if !strings.EqualFold(owner, value) {
				return false
			}
		case "author":
			if value == "@me" {
				value = strings.ToLower(login)
			}
			if !strings.EqualFold(t.Author, value) {
				return false
			}
		case "state":
			if !strings.EqualFold(t.State, value) {
				return false
			}
		case "text":
			if !strings.Contains(strings.ToLower(t.Title), value) {
				return false
			}
		case "reason":
			if ReasonName(value) != ReasonName(t.Reason) {
				return false
			}
		case "is":
			if value == "read" && t.Unread || value == "unread" && !t.Unread {
				return false
			}
			if value != "read" && value != "unread" && !matchesType(value, t.Type) {
				return false
			}
		case "since", "before":
			when, _ := ParseDate(term.Value)
			if term.Field == "since" && !t.UpdatedAt.After(when) ||
				term.Field == "before" && !t.UpdatedAt.Before(when) {
				return false
			}
		}
	}
	return true
}

func matchesType(filter, kind string) bool {
	filter, kind = TypeName(filter), TypeName(kind)
	if filter == "issue-or-pull-request" {
		return kind == "issue" || kind == "pull-request"
	}
	return filter == kind
}

func splitQuery(s string) ([]string, error) {
	var out []string
	var word strings.Builder
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case unicode.IsSpace(r):
			if word.Len() > 0 {
				out = append(out, word.String())
				word.Reset()
			}
		default:
			word.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote in query")
	}
	if word.Len() > 0 {
		out = append(out, word.String())
	}
	return out, nil
}
