package notifs

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
)

type Thread struct {
	ID         string    `json:"id"`
	Repository string    `json:"repository"`
	Title      string    `json:"title"`
	Type       string    `json:"type"`
	Reason     string    `json:"reason"`
	Unread     bool      `json:"unread"`
	UpdatedAt  time.Time `json:"updated_at"`
	LastReadAt time.Time `json:"last_read_at"`
	SubjectURL string    `json:"subject_url"`
	URL        string    `json:"url"`
	Number     int       `json:"number,omitempty"`
	Author     string    `json:"author,omitempty"`
	State      string    `json:"state,omitempty"`
}

func (t Thread) Reference() string {
	if t.Number > 0 {
		return fmt.Sprintf("%s#%d", t.Repository, t.Number)
	}
	return t.Repository
}

// TypeName provides the same local name for query filters and display.
func TypeName(raw string) string {
	name := ReasonName(raw)
	switch strings.ReplaceAll(name, "-", "") {
	case "pr", "pullrequest":
		return "pull-request"
	case "checksuite":
		return "check-suite"
	case "repositoryinvitation":
		return "repository-invitation"
	case "repositoryvulnerabilityalert":
		return "repository-vulnerability-alert"
	case "repositoryadvisory":
		return "repository-advisory"
	default:
		return name
	}
}

func ReasonName(raw string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(raw), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}), "-")
}

func Sort(items []Thread) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
}

// Plain removes terminal controls from remote content, including bidi controls.
func Plain(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}
		return r
	}, s)
}

func InboxURL(host, repo string) string {
	return "https://" + host + "/notifications?query=" + url.QueryEscape("repo:"+repo)
}

type Result struct {
	Items     []Thread
	More      bool
	CheckedAt time.Time
	NextPoll  time.Time
	FromCache bool
	Pages     int
}

type Preview struct {
	Title string
	Body  string
	URL   string
}
