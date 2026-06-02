package bitbucket_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	bbwebhooks "github.com/go-playground/webhooks/v6/bitbucket"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider/bitbucket"
)

const testUUID = "550e8400-e29b-41d4-a716-446655440000"
const testAfter = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func roundTrip(t *testing.T, headers http.Header, body []byte, uuid string) bbwebhooks.RepoPushPayload {
	t.Helper()
	var opts []bbwebhooks.Option
	if uuid != "" {
		opts = append(opts, bbwebhooks.Options.UUID(uuid))
	}
	hook, err := bbwebhooks.New(opts...)
	if err != nil {
		t.Fatalf("bbwebhooks.New: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	payload, err := hook.Parse(req, bbwebhooks.RepoPushEvent)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	pl, ok := payload.(bbwebhooks.RepoPushPayload)
	if !ok {
		t.Fatalf("expected RepoPushPayload, got %T", payload)
	}
	return pl
}

func TestBuildWebhook_Push_RoundTrip(t *testing.T) {
	p := bitbucket.Provider{}
	event := core.PushEvent{
		Host:   "bitbucket.org",
		Owner:  "acme",
		Repo:   "widget",
		Branch: "main",
		Before: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		After:  testAfter,
	}

	headers, body, err := p.BuildWebhook(event, testUUID)
	if err != nil {
		t.Fatalf("BuildWebhook: %v", err)
	}
	if got := headers.Get("X-Event-Key"); got != "repo:push" {
		t.Fatalf("X-Event-Key: want repo:push, got %q", got)
	}
	if got := headers.Get("X-Hook-UUID"); got != testUUID {
		t.Fatalf("X-Hook-UUID: want %q, got %q", testUUID, got)
	}

	pl := roundTrip(t, headers, body, testUUID)

	wantURL := p.RepoURL("bitbucket.org", "acme", "widget")
	if pl.Repository.Links.HTML.Href != wantURL {
		t.Errorf("Repository.Links.HTML.Href: want %q, got %q", wantURL, pl.Repository.Links.HTML.Href)
	}
	if len(pl.Push.Changes) == 0 {
		t.Fatal("Push.Changes is empty")
	}
	if got := pl.Push.Changes[0].New.Target.Hash; got != testAfter {
		t.Errorf("Push.Changes[0].New.Target.Hash: want %q, got %q", testAfter, got)
	}
	if got := pl.Push.Changes[0].New.Name; got != "main" {
		t.Errorf("Push.Changes[0].New.Name: want main, got %q", got)
	}
}

func TestBuildWebhook_WrongUUID(t *testing.T) {
	p := bitbucket.Provider{}
	event := core.PushEvent{
		Host: "bitbucket.org", Owner: "acme", Repo: "w", Branch: "main", After: testAfter,
	}

	headers, body, err := p.BuildWebhook(event, "correct-uuid")
	if err != nil {
		t.Fatal(err)
	}

	hook, _ := bbwebhooks.New(bbwebhooks.Options.UUID("different-uuid"))
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	_, err = hook.Parse(req, bbwebhooks.RepoPushEvent)
	if err == nil {
		t.Fatal("expected UUID-mismatch error, got nil")
	}
}

func TestBuildWebhook_NoUUID(t *testing.T) {
	p := bitbucket.Provider{}
	event := core.PushEvent{
		Host: "bitbucket.org", Owner: "acme", Repo: "w", Branch: "main", After: testAfter,
	}
	// secret="" → no X-Hook-UUID header; parser with no uuid configured still accepts it.
	headers, body, err := p.BuildWebhook(event, "")
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("X-Hook-UUID") != "" {
		t.Fatal("X-Hook-UUID must be absent when uuid is empty")
	}
	roundTrip(t, headers, body, "") // must not error
}

func TestRepoURL(t *testing.T) {
	p := bitbucket.Provider{}
	got := p.RepoURL("bitbucket.org", "acme", "widget")
	want := "https://bitbucket.org/acme/widget"
	if got != want {
		t.Fatalf("RepoURL: want %q, got %q", want, got)
	}
}
