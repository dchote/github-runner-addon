package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to the GitHub (or GHES) REST API for runner registration.
type Client struct {
	PAT        string
	HTTPClient *http.Client
}

func New(pat string) *Client {
	return &Client{
		PAT: strings.TrimSpace(pat),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) Configured() bool {
	return c != nil && c.PAT != ""
}

// ScopeInfo is derived from a GitHub project URL.
type ScopeInfo struct {
	Scope   string // repo | org
	Owner   string
	Repo    string // empty for org
	APIBase string // e.g. https://api.github.com or https://ghes.example.com/api/v3
	HTMLURL string // normalized project URL without trailing slash
}

// OrgName returns the organization name for org-scoped runners (Owner).
func (s ScopeInfo) OrgName() string {
	if s.Scope == "org" {
		return s.Owner
	}
	return ""
}

// ParseProjectURL derives scope and API base from an org or repo HTML URL.
func ParseProjectURL(raw string) (ScopeInfo, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.Scheme == "" {
		return ScopeInfo{}, fmt.Errorf("invalid url")
	}
	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		return ScopeInfo{}, fmt.Errorf("url must include owner or owner/repo")
	}
	if len(parts) > 2 {
		return ScopeInfo{}, fmt.Errorf("url must be https://host/owner or https://host/owner/repo")
	}
	info := ScopeInfo{
		Owner:   parts[0],
		HTMLURL: strings.TrimRight(strings.TrimSpace(raw), "/"),
	}
	if len(parts) == 1 {
		info.Scope = "org"
	} else {
		info.Scope = "repo"
		info.Repo = parts[1]
	}
	host := strings.ToLower(u.Hostname())
	if host == "github.com" || host == "www.github.com" {
		info.APIBase = "https://api.github.com"
	} else {
		info.APIBase = strings.TrimRight(fmt.Sprintf("%s://%s/api/v3", u.Scheme, u.Host), "/")
	}
	return info, nil
}

func (s ScopeInfo) runnersBasePath() string {
	if s.Scope == "org" {
		return fmt.Sprintf("%s/orgs/%s/actions/runners", s.APIBase, url.PathEscape(s.Owner))
	}
	return fmt.Sprintf("%s/repos/%s/%s/actions/runners", s.APIBase, url.PathEscape(s.Owner), url.PathEscape(s.Repo))
}

type registrationTokenResponse struct {
	Token string `json:"token"`
}

// MintRegistrationToken creates a short-lived Actions runner registration token.
func (c *Client) MintRegistrationToken(ctx context.Context, projectURL string) (string, error) {
	if !c.Configured() {
		return "", fmt.Errorf("github PAT not configured")
	}
	info, err := ParseProjectURL(projectURL)
	if err != nil {
		return "", err
	}
	var out registrationTokenResponse
	if err := c.doJSON(ctx, http.MethodPost, info.runnersBasePath()+"/registration-token", nil, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", fmt.Errorf("empty registration token from GitHub")
	}
	return out.Token, nil
}

type runnerListResponse struct {
	TotalCount int `json:"total_count"`
	Runners    []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"runners"`
}

// FindRunnerID looks up a self-hosted runner by name under the project URL (paginated).
func (c *Client) FindRunnerID(ctx context.Context, projectURL, runnerName string) (int64, error) {
	if !c.Configured() {
		return 0, fmt.Errorf("github PAT not configured")
	}
	info, err := ParseProjectURL(projectURL)
	if err != nil {
		return 0, err
	}
	base := info.runnersBasePath()
	page := 1
	for {
		path := base + "?per_page=100&page=" + strconv.Itoa(page)
		var out runnerListResponse
		if err := c.doJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
			return 0, err
		}
		for _, r := range out.Runners {
			if r.Name == runnerName {
				return r.ID, nil
			}
		}
		if len(out.Runners) < 100 {
			break
		}
		page++
		if page > 50 {
			break
		}
	}
	return 0, fmt.Errorf("runner %q not found on GitHub", runnerName)
}

// DeregisterRunner removes a runner from GitHub by name (best-effort lookup + delete).
func (c *Client) DeregisterRunner(ctx context.Context, projectURL, runnerName string) error {
	id, err := c.FindRunnerID(ctx, projectURL, runnerName)
	if err != nil {
		return err
	}
	return c.DeleteRunner(ctx, projectURL, id)
}

// DeleteRunner deletes a runner by numeric GitHub id.
func (c *Client) DeleteRunner(ctx context.Context, projectURL string, runnerID int64) error {
	if !c.Configured() {
		return fmt.Errorf("github PAT not configured")
	}
	info, err := ParseProjectURL(projectURL)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("%s/%d", info.runnersBasePath(), runnerID)
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

func (c *Client) doJSON(ctx context.Context, method, urlStr string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.PAT)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("github api %s %s: %s", method, urlStr, truncate(msg, 300))
	}
	if out == nil || len(data) == 0 || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.Unmarshal(data, out)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
