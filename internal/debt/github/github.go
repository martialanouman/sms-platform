// Package github reads the open debt issues of a repository from the GitHub REST API.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// Client lists the open issues labelled debt of Repo ("owner/name").
type Client struct {
	BaseURL string
	Repo    string
	Token   string
	HTTP    *http.Client
}

// OpenDebt returns the numbers of the open issues labelled debt, pull requests excluded.
func (c Client) OpenDebt(ctx context.Context) (map[int]bool, error) {
	open := map[int]bool{}
	for page := 1; ; page++ {
		issues, err := c.page(ctx, page)
		if err != nil {
			return nil, err
		}
		if len(issues) == 0 {
			return open, nil
		}
		for _, i := range issues {
			if i.PullRequest == nil {
				open[i.Number] = true
			}
		}
	}
}

type issue struct {
	Number      int       `json:"number"`
	PullRequest *struct{} `json:"pull_request"`
}

func (c Client) page(ctx context.Context, page int) ([]issue, error) {
	q := url.Values{"labels": {"debt"}, "state": {"open"}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/repos/"+c.Repo+"/issues?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("issues of %s, page %d: %s", c.Repo, page, resp.Status)
	}
	var issues []issue
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return nil, fmt.Errorf("issues of %s, page %d: %w", c.Repo, page, err)
	}
	return issues, nil
}
