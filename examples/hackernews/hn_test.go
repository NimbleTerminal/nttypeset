package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStoriesPreserveRankAndSkipUnavailableItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/topstories.json":
			_, _ = fmt.Fprint(w, `[1,2,3,4]`)
		case "/v0/item/1.json":
			time.Sleep(20 * time.Millisecond)
			_, _ = fmt.Fprint(w, `{"id":1,"type":"story","title":"first"}`)
		case "/v0/item/2.json":
			_, _ = fmt.Fprint(w, `{"id":2,"type":"story","title":"second","url":"https://example.com"}`)
		case "/v0/item/3.json":
			_, _ = fmt.Fprint(w, `{"id":3,"type":"comment","title":"not a story"}`)
		case "/v0/item/4.json":
			_, _ = fmt.Fprint(w, `{"id":4,"type":"story","title":"deleted","deleted":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &hnClient{baseURL: server.URL + "/v0", http: server.Client()}
	stories, err := client.stories(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(stories) != 2 || stories[0].ID != 1 || stories[1].ID != 2 {
		t.Fatalf("stories = %+v, want ranked IDs 1 and 2", stories)
	}
	if got := stories[0].storyURL(); got != "https://news.ycombinator.com/item?id=1" {
		t.Fatalf("URL-less story = %q", got)
	}
}

func TestStoriesLimitCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v0/topstories.json" {
			_, _ = fmt.Fprint(w, `[1,2]`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":%s,"type":"story","title":"story"}`, r.URL.Path[len("/v0/item/"):len(r.URL.Path)-len(".json")])
	}))
	defer server.Close()
	client := &hnClient{baseURL: server.URL + "/v0", http: server.Client()}
	stories, err := client.stories(context.Background(), 1)
	if err != nil || len(stories) != 1 || stories[0].ID != 1 {
		t.Fatalf("stories = %+v, err = %v", stories, err)
	}
}

func TestStoriesReportHTTPAndJSONErrors(t *testing.T) {
	for _, test := range []struct {
		name, top, item string
		status          int
	}{
		{name: "top status", status: http.StatusBadGateway},
		{name: "top JSON", top: `[`, status: http.StatusOK},
		{name: "item JSON", top: `[1]`, item: `{`, status: http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				if r.URL.Path == "/v0/topstories.json" {
					_, _ = fmt.Fprint(w, test.top)
				} else {
					_, _ = fmt.Fprint(w, test.item)
				}
			}))
			defer server.Close()
			client := &hnClient{baseURL: server.URL + "/v0", http: server.Client()}
			if _, err := client.stories(context.Background(), 1); err == nil {
				t.Fatal("no error")
			}
		})
	}
}

func TestStoryURLRejectsNonWebSchemes(t *testing.T) {
	s := story{ID: 42, URL: "javascript:alert(1)"}
	if got := s.storyURL(); got != s.discussionURL() {
		t.Fatalf("storyURL = %q, want %q", got, s.discussionURL())
	}
}
