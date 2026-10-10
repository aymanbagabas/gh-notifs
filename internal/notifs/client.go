package notifs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"
)

type Client struct {
	HTTP    *http.Client
	BaseURL string
	GraphQL string
	Host    string
	Cache   *Cache

	initMu       sync.Mutex
	cacheRoot    string
	identityKey  string
	mu           sync.Mutex
	accountID    string
	authorMu     sync.Mutex
	authors      map[string]string
	blockedUntil time.Time
	details      map[string]Thread
}

func NewClient(host string) (*Client, error) {
	token, _ := auth.TokenForHost(host)
	if token == "" {
		return nil, fmt.Errorf("no GitHub credentials for %s; run gh auth login", host)
	}
	httpClient, err := api.NewHTTPClient(api.ClientOptions{Host: host, AuthToken: token,
		Headers: map[string]string{"X-GitHub-Api-Version": "2022-11-28"},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}})
	if err != nil {
		return nil, err
	}
	root, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	base := "https://api." + host + "/"
	graphql := base + "graphql"
	if auth.IsEnterprise(host) {
		base = "https://" + host + "/api/v3/"
		graphql = "https://" + host + "/api/graphql"
	}
	c := &Client{HTTP: httpClient, BaseURL: base, GraphQL: graphql, Host: host,
		cacheRoot: root, identityKey: cacheKey(host, token), details: make(map[string]Thread)}
	if _, err := c.loadIdentity(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) BlockedUntil() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blockedUntil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (c *Client) request(ctx context.Context, method, endpoint string, body io.Reader, headers http.Header) (*http.Response, error) {
	if wait := time.Until(c.BlockedUntil()); wait > 0 {
		return nil, fmt.Errorf("GitHub rate limit: try again after %s", c.BlockedUntil().Format(time.RFC3339))
	}
	if !strings.HasPrefix(endpoint, "https://") && !strings.HasPrefix(endpoint, "http://") {
		endpoint = c.BaseURL + endpoint
	}
	target, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, err
	}
	if target.Scheme != base.Scheme || target.Host != base.Host {
		return nil, fmt.Errorf("refusing API request to another host")
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	for k, values := range headers {
		req.Header[k] = values
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if resp.StatusCode == 429 || resp.StatusCode == 403 {
		until := time.Now().Add(time.Minute)
		if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
			until = time.Now().Add(time.Duration(seconds) * time.Second)
		} else if parsed, err := http.ParseTime(resp.Header.Get("Retry-After")); err == nil {
			until = parsed
		}
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			if epoch, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
				until = maxTime(until, time.Unix(epoch, 0))
			}
		}
		if resp.StatusCode == 429 || resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0" {
			c.blockedUntil = until
		}
	}
	c.mu.Unlock()
	if resp.StatusCode == http.StatusNotModified || resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer resp.Body.Close()
	return nil, api.HandleHTTPError(resp)
}

// Endpoint builds one request. Split a user query with Query.Requests first.
func Endpoint(q Query, page int) string {
	path := "notifications"
	values := url.Values{"all": {"true"}, "per_page": {"50"}, "page": {strconv.Itoa(page)}}
	states := q.values("is")
	if slices.Contains(states, "unread") && !slices.Contains(states, "read") {
		values.Set("all", "false")
	}
	if q.participatingOnly() {
		values.Set("participating", "true")
	}
	bounds := make(map[string]time.Time)
	for _, term := range q.Terms {
		switch term.Field {
		case "repo":
			owner, repo, _ := strings.Cut(term.Value, "/")
			path = "repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/notifications"
			values.Set("per_page", "100")
		case "since", "before":
			when, _ := ParseDate(term.Value)
			bound, ok := bounds[term.Field]
			if !ok || term.Field == "since" && when.After(bound) || term.Field == "before" && when.Before(bound) {
				bounds[term.Field] = when
			}
		}
	}
	for field, when := range bounds {
		// REST documents whole seconds; widen before and let the local filter
		// enforce the original fractional bound.
		if field == "before" && when.Nanosecond() != 0 {
			when = when.Truncate(time.Second).Add(time.Second)
		}
		values.Set(field, when.UTC().Format(time.RFC3339))
	}
	return path + "?" + values.Encode()
}

func (c *Client) page(ctx context.Context, endpoint string, details map[string]Thread) (Page, error) {
	generation := c.Cache.Generation()
	cached, ok := c.Cache.Get(endpoint)
	headers := make(http.Header)
	if ok {
		if cached.ETag != "" {
			headers.Set("If-None-Match", cached.ETag)
		}
		if cached.LastModified != "" {
			headers.Set("If-Modified-Since", cached.LastModified)
		}
	}
	resp, err := c.request(ctx, http.MethodGet, endpoint, nil, headers)
	if err != nil {
		return Page{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		if !ok {
			return Page{}, fmt.Errorf("GitHub returned 304 without a cached page")
		}
		for i := range cached.Items {
			if detail, ok := details[detailKey(cached.Items[i])]; ok {
				cached.Items[i].copyDetails(detail)
			}
		}
		cached.CheckedAt = time.Now()
		c.Cache.PutIfCurrent(endpoint, cached, generation)
		return cached, nil
	}
	var raw []struct {
		ID         string    `json:"id"`
		Unread     bool      `json:"unread"`
		Reason     string    `json:"reason"`
		UpdatedAt  time.Time `json:"updated_at"`
		LastReadAt time.Time `json:"last_read_at"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Subject struct {
			Title string `json:"title"`
			Type  string `json:"type"`
			URL   string `json:"url"`
		} `json:"subject"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Page{}, err
	}
	p := Page{Next: strings.Contains(resp.Header.Get("Link"), `rel="next"`),
		ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified"), CheckedAt: time.Now()}
	for _, r := range raw {
		item := Thread{ID: r.ID, Repository: r.Repository.FullName, Title: r.Subject.Title,
			Type: r.Subject.Type, SubjectURL: r.Subject.URL, Unread: r.Unread, Reason: r.Reason,
			UpdatedAt: r.UpdatedAt, LastReadAt: r.LastReadAt, URL: InboxURL(c.Host, r.Repository.FullName)}
		if item.Type == "Issue" || item.Type == "PullRequest" {
			if u, err := url.Parse(item.SubjectURL); err == nil {
				parts := strings.Split(strings.TrimSuffix(u.Path, "/"), "/")
				if len(parts) > 0 {
					item.Number, _ = strconv.Atoi(parts[len(parts)-1])
				}
			}
			if item.Number > 0 {
				kind := "issues"
				if item.Type == "PullRequest" {
					kind = "pull"
				}
				item.URL = fmt.Sprintf("https://%s/%s/%s/%d", c.Host, item.Repository, kind, item.Number)
			}
		}
		if detail, ok := details[detailKey(item)]; ok {
			item.copyDetails(detail)
		}
		p.Items = append(p.Items, item)
	}
	c.Cache.PutIfCurrent(endpoint, p, generation)
	return p, nil
}

func (c *Client) RecentRepositories() []Repository {
	c.mu.Lock()
	cache := c.Cache
	c.mu.Unlock()
	if cache == nil {
		return nil
	}
	return cache.RecentRepositories()
}

func (c *Client) Cached(q Query, limit int) Result {
	c.mu.Lock()
	cache := c.Cache
	c.mu.Unlock()
	if cache == nil || q.NeedsDetails() {
		return Result{FromCache: true}
	}
	var results []Result
	for _, request := range q.Requests() {
		results = append(results, cachedRequest(cache, request, limit))
	}
	return mergeResults(results, limit)
}

func cachedRequest(cache *Cache, q Query, limit int) Result {
	result := Result{FromCache: true}
	seen := make(map[string]bool)
	participating := q.participatingOnly()
	for n := 1; ; n++ {
		p, ok := cache.Get(Endpoint(q, n))
		if !ok {
			result.More = true
			break
		}
		if result.CheckedAt.IsZero() || p.CheckedAt.Before(result.CheckedAt) {
			result.CheckedAt = p.CheckedAt
		}
		for _, item := range p.Items {
			if !seen[item.ID] && q.match(item, "", participating) {
				seen[item.ID] = true
				result.Items = append(result.Items, item)
			}
		}
		result.Pages = n
		result.More = p.Next || len(result.Items) > limit
		if !p.Next || len(result.Items) >= limit {
			break
		}
	}
	Sort(result.Items)
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
	}
	return result
}

func (c *Client) Fetch(ctx context.Context, q Query, limit int, progress func(Result)) (result Result, err error) {
	if err := c.Initialize(ctx); err != nil {
		return result, err
	}
	if err := c.Cache.refresh(); err != nil {
		return result, fmt.Errorf("refresh metadata cache: %w", err)
	}
	details := c.Cache.detailSnapshot()
	c.mu.Lock()
	for key, detail := range c.details {
		details[key] = detail
	}
	c.mu.Unlock()
	defer func() {
		if saveErr := c.Cache.Save(); saveErr != nil {
			err = errors.Join(err, fmt.Errorf("save metadata cache: %w", saveErr))
		}
	}()
	q.authorIDs, err = c.resolveAuthors(ctx, q)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	requests := q.Requests()
	results := make([]Result, len(requests))
	errs := make([]error, len(requests))
	for i := range results {
		results[i].More = true
	}
	var mu sync.Mutex
	update := func(i int, r Result) {
		mu.Lock()
		defer mu.Unlock()
		results[i] = r
		if progress != nil {
			progress(mergeResults(results, limit))
		}
	}
	var wg sync.WaitGroup
	for i, request := range requests {
		wg.Go(func() {
			r, err := c.fetchRequest(ctx, request, limit, details, func(r Result) { update(i, r) })
			if err != nil {
				errs[i] = fmt.Errorf("%s: %w", Endpoint(request, 1), err)
				r.More = true
				cancel()
			}
			mu.Lock()
			results[i] = r
			mu.Unlock()
		})
	}
	wg.Wait()
	return mergeResults(results, limit), errors.Join(errs...)
}

func mergeResults(results []Result, limit int) Result {
	merged := Result{FromCache: true}
	for _, result := range results {
		merged.Items = append(merged.Items, result.Items...)
		merged.More = merged.More || result.More
		merged.FromCache = merged.FromCache && result.FromCache
		merged.Pages += result.Pages
		if !result.CheckedAt.IsZero() && (merged.CheckedAt.IsZero() || result.CheckedAt.Before(merged.CheckedAt)) {
			merged.CheckedAt = result.CheckedAt
		}
	}
	Sort(merged.Items)
	seen := make(map[string]bool)
	items := merged.Items[:0]
	for _, item := range merged.Items {
		if !seen[item.ID] {
			seen[item.ID] = true
			items = append(items, item)
		}
	}
	merged.More = merged.More || len(items) > limit
	merged.Items = items[:min(limit, len(items))]
	return merged
}

func (c *Client) fetchRequest(ctx context.Context, q Query, limit int, details map[string]Thread, progress func(Result)) (result Result, err error) {
	seen := make(map[string]bool)
	participating := q.participatingOnly()
	for n := 1; ; n++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		p, err := c.page(ctx, Endpoint(q, n), details)
		if err != nil {
			return result, err
		}
		if q.NeedsDetails() {
			if err := c.Enrich(ctx, p.Items); err != nil {
				return result, err
			}
		}
		for _, item := range p.Items {
			if !seen[item.ID] && q.match(item, "", participating) {
				seen[item.ID] = true
				result.Items = append(result.Items, item)
			}
		}
		Sort(result.Items)
		result.More = p.Next || len(result.Items) > limit
		if len(result.Items) > limit {
			result.Items = result.Items[:limit]
		}
		result.Pages = n
		if progress != nil {
			copyResult := result
			copyResult.Items = append([]Thread(nil), result.Items...)
			progress(copyResult)
		}
		if !p.Next || len(result.Items) >= limit {
			break
		}
	}
	result.CheckedAt = time.Now()
	return result, nil
}

func detailKey(t Thread) string {
	return t.Repository + "#" + strconv.Itoa(t.Number) + ":" + t.Type + ":" + t.SubjectURL + ":" + t.UpdatedAt.Format(time.RFC3339Nano)
}

func (c *Client) Enrich(ctx context.Context, items []Thread) error {
	c.mu.Lock()
	cache := c.Cache
	c.mu.Unlock()
	var generation uint64
	if cache != nil {
		generation = cache.Generation()
	}
	var missing []int
	for i := range items {
		if items[i].Number == 0 || (items[i].Type != "Issue" && items[i].Type != "PullRequest") {
			continue
		}
		c.mu.Lock()
		detail, ok := c.details[detailKey(items[i])]
		c.mu.Unlock()
		if ok {
			items[i].copyDetails(detail)
		} else {
			missing = append(missing, i)
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan []int)
	errs := make(chan error, 4)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for batch := range jobs {
				if err := c.enrichBatch(ctx, items, batch); err != nil {
					errs <- err
					cancel()
					return
				}
			}
		})
	}
send:
	for start := 0; start < len(missing); start += 25 {
		select {
		case jobs <- missing[start:min(start+25, len(missing))]:
		case <-ctx.Done():
			break send
		}
	}
	close(jobs)
	wg.Wait()
	close(errs)
	var err error
	for e := range errs {
		err = errors.Join(err, e)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && cache != nil {
		if saveErr := cache.storeDetails(items, generation); saveErr != nil {
			err = fmt.Errorf("save notification details: %w", saveErr)
		}
	}
	return err
}

func (c *Client) enrichBatch(ctx context.Context, items []Thread, batch []int) error {
	var declarations, fields []string
	vars := make(map[string]any)
	for n, index := range batch {
		owner, repo, _ := strings.Cut(items[index].Repository, "/")
		declarations = append(declarations, fmt.Sprintf("$o%d:String!,$r%d:String!,$n%d:Int!", n, n, n))
		vars[fmt.Sprintf("o%d", n)], vars[fmt.Sprintf("r%d", n)], vars[fmt.Sprintf("n%d", n)] = owner, repo, items[index].Number
		fields = append(fields, fmt.Sprintf(`v%d:repository(owner:$o%d,name:$r%d){issueOrPullRequest(number:$n%d){
... on Issue {state author{login ... on Node{id}} url}
... on PullRequest {state isDraft author{login ... on Node{id}} url}
}}`, n, n, n, n))
	}
	payload, err := json.Marshal(map[string]any{"query": "query(" + strings.Join(declarations, ",") + "){" + strings.Join(fields, "\n") + "}", "variables": vars})
	if err != nil {
		return err
	}
	resp, err := c.request(ctx, http.MethodPost, c.GraphQL, bytes.NewReader(payload), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var data struct {
		Data map[string]*struct {
			Item *struct {
				State   string
				IsDraft bool
				URL     string
				Author  struct{ ID, Login string }
			} `json:"issueOrPullRequest"`
		}
		Errors []struct{ Message string }
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}
	if len(data.Errors) > 0 {
		return fmt.Errorf("GraphQL details: %s", data.Errors[0].Message)
	}
	for n, index := range batch {
		repo := data.Data[fmt.Sprintf("v%d", n)]
		if repo == nil || repo.Item == nil {
			return fmt.Errorf("GitHub did not return details for %s", items[index].Reference())
		}
		d := repo.Item
		items[index].Author, items[index].State, items[index].URL = d.Author.Login, strings.ToLower(d.State), d.URL
		items[index].AuthorID = d.Author.ID
		if d.IsDraft && d.State == "OPEN" {
			items[index].State = "draft"
		}
		c.mu.Lock()
		if c.details == nil {
			c.details = make(map[string]Thread)
		}
		c.details[detailKey(items[index])] = items[index]
		c.mu.Unlock()
	}
	return nil
}

func (c *Client) Preview(ctx context.Context, t Thread) (Preview, error) {
	if t.SubjectURL == "" {
		return Preview{Title: t.Title, URL: t.URL, Body: "GitHub provides no public detail endpoint for this notification."}, nil
	}
	resp, err := c.request(ctx, http.MethodGet, t.SubjectURL, nil, nil)
	if err != nil {
		return Preview{}, err
	}
	defer resp.Body.Close()
	var d struct {
		Title   string                   `json:"title"`
		Name    string                   `json:"name"`
		Body    string                   `json:"body"`
		HTMLURL string                   `json:"html_url"`
		Commit  struct{ Message string } `json:"commit"`
		User    *previewActor            `json:"user"`
		Author  *previewActor            `json:"author"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return Preview{}, err
	}
	title, body, web := t.Title, d.Body, t.URL
	if d.Title != "" {
		title = d.Title
	}
	if body == "" {
		body = d.Commit.Message
	}
	if body == "" {
		body = "No text preview is available. Open this notification in the browser."
	}
	if d.HTMLURL != "" {
		web = d.HTMLURL
	}
	preview := Preview{Title: title, Body: body, URL: web}
	if t.Number > 0 && (t.Type == "Issue" || t.Type == "PullRequest") {
		t.URL = web
		preview.Metadata, err = c.previewMetadata(ctx, t)
		if err != nil {
			return preview, fmt.Errorf("preview metadata incomplete: %w", err)
		}
	} else if d.User != nil {
		preview.Metadata = []PreviewSection{{Name: "Author", Items: []string{actorName(d.User)}, Total: -1}}
	} else if d.Author != nil {
		preview.Metadata = []PreviewSection{{Name: "Author", Items: []string{actorName(d.Author)}, Total: -1}}
	}
	return preview, nil
}

func (c *Client) Action(ctx context.Context, id, action string) error {
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return fmt.Errorf("invalid notification thread ID %q", id)
	}
	method, endpoint := "", "notifications/threads/"+id
	var body io.Reader
	switch action {
	case "read":
		method = http.MethodPatch
	case "done":
		method = http.MethodDelete
	case "unsubscribe":
		method, endpoint, body = http.MethodPut, endpoint+"/subscription", strings.NewReader(`{"ignored":true}`)
	case "subscribe":
		method, endpoint, body = http.MethodPut, endpoint+"/subscription", strings.NewReader(`{"ignored":false}`)
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
	if err := c.Initialize(ctx); err != nil {
		return err
	}
	resp, err := c.request(ctx, method, endpoint, body, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return c.Cache.Invalidate()
}
