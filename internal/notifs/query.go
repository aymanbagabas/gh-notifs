package notifs

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

type Term struct {
	Field string
	Value string
}

type Query struct {
	Raw       string
	Terms     []Term
	authorIDs map[string]string
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
			case "read", "unread", "issue", "pull-request", "issue-or-pull-request",
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
	return q.match(t, login, false)
}

// participating records membership in a participating=true response, not a reason.
func (q Query) match(t Thread, login string, participating bool) bool {
	groups := make(map[string]bool)
	for _, term := range q.Terms {
		value := strings.ToLower(term.Value)
		group, matched := term.Field, false
		switch term.Field {
		case "repo":
			matched = strings.EqualFold(t.Repository, value)
		case "org":
			owner, _, _ := strings.Cut(t.Repository, "/")
			matched = strings.EqualFold(owner, value)
		case "author":
			if q.authorIDs != nil {
				matched = t.AuthorID != "" && t.AuthorID == q.authorIDs[value]
				break
			}
			if value == "@me" {
				value = strings.ToLower(login)
			}
			matched = strings.EqualFold(t.Author, value)
		case "state":
			matched = strings.EqualFold(t.State, value)
		case "text":
			if !strings.Contains(strings.ToLower(t.Title), value) {
				return false
			}
			continue
		case "reason":
			if ReasonName(value) == "participating" {
				matched = participating
			} else {
				matched = ReasonName(value) == ReasonName(t.Reason)
			}
		case "is":
			if value == "read" || value == "unread" {
				group = "read-status"
				matched = (value == "unread") == t.Unread
			} else {
				matched = matchesType(value, t.Type)
			}
		case "since", "before":
			when, _ := ParseDate(term.Value)
			if term.Field == "since" && !t.UpdatedAt.After(when) ||
				term.Field == "before" && !t.UpdatedAt.Before(when) {
				return false
			}
			continue
		}
		groups[group] = groups[group] || matched
	}
	for _, matched := range groups {
		if !matched {
			return false
		}
	}
	return true
}

func (q Query) values(field string) []string {
	var values []string
	for _, term := range q.Terms {
		if term.Field != field {
			continue
		}
		value := strings.ToLower(term.Value)
		if field == "reason" {
			value = ReasonName(value)
		}
		if !slices.Contains(values, value) {
			values = append(values, value)
		}
	}
	return values
}

func (q Query) participatingOnly() bool {
	reasons := q.values("reason")
	for _, reason := range reasons {
		switch reason {
		case "participating", "author", "comment", "mention":
		default:
			return false
		}
	}
	return len(reasons) > 0
}

// Requests splits repository and aggregate-reason alternatives into API queries.
// Each returned query owns its terms and can use Endpoint independently.
func (q Query) Requests() []Query {
	repos := q.values("repo")
	if len(repos) == 0 {
		repos = []string{""}
	}
	splitReasons := slices.Contains(q.values("reason"), "participating") && !q.participatingOnly()
	var requests []Query
	for _, repo := range repos {
		request := Query{Raw: q.Raw, authorIDs: q.authorIDs}
		for _, term := range q.Terms {
			if term.Field != "repo" {
				request.Terms = append(request.Terms, term)
			}
		}
		if repo != "" {
			request.Terms = append(request.Terms, Term{Field: "repo", Value: repo})
		}
		if !splitReasons {
			requests = append(requests, request)
			continue
		}
		participating, other := Query{Raw: q.Raw, authorIDs: q.authorIDs}, Query{Raw: q.Raw, authorIDs: q.authorIDs}
		for _, term := range request.Terms {
			if term.Field != "reason" {
				participating.Terms = append(participating.Terms, term)
				other.Terms = append(other.Terms, term)
			} else if ReasonName(term.Value) != "participating" {
				other.Terms = append(other.Terms, term)
			}
		}
		participating.Terms = append(participating.Terms, Term{Field: "reason", Value: "participating"})
		requests = append(requests, participating, other)
	}
	return requests
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
