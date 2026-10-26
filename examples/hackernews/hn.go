package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
)

type hnClient struct {
	baseURL string
	http    *http.Client
}

type story struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	By          string `json:"by"`
	URL         string `json:"url"`
	Score       int    `json:"score"`
	Descendants int    `json:"descendants"`
	Time        int64  `json:"time"`
	Deleted     bool   `json:"deleted"`
	Dead        bool   `json:"dead"`
}

func (s story) discussionURL() string {
	return "https://news.ycombinator.com/item?id=" + strconv.Itoa(s.ID)
}

func (s story) storyURL() string {
	u, err := url.Parse(s.URL)
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return u.String()
	}
	return s.discussionURL()
}

func (c *hnClient) stories(ctx context.Context, count int) ([]story, error) {
	var ids []int
	if err := c.get(ctx, "/topstories.json", &ids); err != nil {
		return nil, fmt.Errorf("fetch top stories: %w", err)
	}
	ids = ids[:min(count, len(ids))]
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	items := make([]story, len(ids))
	jobs := make(chan int, len(ids))
	for i := range ids {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	var once sync.Once
	var fetchErr error
	for range min(8, len(ids)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				var item story
				if err := c.get(ctx, "/item/"+strconv.Itoa(ids[i])+".json", &item); err != nil {
					once.Do(func() {
						fetchErr = fmt.Errorf("fetch story %d: %w", ids[i], err)
						cancel()
					})
					return
				}
				items[i] = item
			}
		}()
	}
	wg.Wait()
	if fetchErr != nil {
		return nil, fetchErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stories := make([]story, 0, len(items))
	for _, item := range items {
		if item.ID != 0 && item.Type == "story" && !item.Deleted && !item.Dead {
			stories = append(stories, item)
		}
	}
	return stories, nil
}

func (c *hnClient) get(ctx context.Context, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
