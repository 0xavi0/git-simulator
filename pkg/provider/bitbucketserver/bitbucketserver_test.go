package bitbucketserver_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	bswebhooks "github.com/go-playground/webhooks/v6/bitbucket-server"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider/bitbucketserver"
)

const (
	testSecret = "bbs-secret"
	testBefore = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testAfter  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func roundTrip(t *testing.T, headers http.Header, body []byte, secret string) bswebhooks.RepositoryReferenceChangedPayload {
	t.Helper()
	var opts []bswebhooks.Option
	if secret != "" {
		opts = append(opts, bswebhooks.Options.Secret(secret))
	}
	hook, err := bswebhooks.New(opts...)
	if err != nil {
		t.Fatalf("bswebhooks.New: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	payload, err := hook.Parse(req, bswebhooks.RepositoryReferenceChangedEvent)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	pl, ok := payload.(bswebhooks.RepositoryReferenceChangedPayload)
	if !ok {
		t.Fatalf("expected RepositoryReferenceChangedPayload, got %T", payload)
	}
	return pl
}

func TestBuildWebhook_Push_RoundTrip(t *testing.T) {
	p := bitbucketserver.Provider{}
	event := core.PushEvent{
		Host:   "bitbucket.example.com",
		Owner:  "acme",
		Repo:   "widget",
		Branch: "main",
		Before: testBefore,
		After:  testAfter,
	}

	headers, body, err := p.BuildWebhook(event, testSecret)
	if err != nil {
		t.Fatalf("BuildWebhook: %v", err)
	}
	if got := headers.Get("X-Event-Key"); got != "repo:refs_changed" {
		t.Fatalf("X-Event-Key: want repo:refs_changed, got %q", got)
	}
	if headers.Get("X-Hook-UUID") != "" {
		t.Fatal("X-Hook-UUID must be absent for Bitbucket Server (detection-order rule)")
	}
	if headers.Get("X-Hub-Signature") == "" {
		t.Fatal("X-Hub-Signature must be present when secret is non-empty")
	}

	pl := roundTrip(t, headers, body, testSecret)

	if len(pl.Changes) == 0 {
		t.Fatal("Changes is empty")
	}
	if pl.Changes[0].ToHash != testAfter {
		t.Errorf("ToHash: want %q, got %q", testAfter, pl.Changes[0].ToHash)
	}
	if pl.Changes[0].ReferenceID != "refs/heads/main" {
		t.Errorf("ReferenceID: want refs/heads/main, got %q", pl.Changes[0].ReferenceID)
	}

	// Verify clone link is present in repository links.
	cloneLinks, ok := pl.Repository.Links["clone"]
	if !ok {
		t.Fatal("repository.links.clone missing")
	}
	links, ok := cloneLinks.([]interface{})
	if !ok || len(links) == 0 {
		t.Fatal("clone links must be a non-empty list")
	}
	wantURL := p.RepoURL("bitbucket.example.com", "acme", "widget")
	found := false
	for _, l := range links {
		m, ok := l.(map[string]interface{})
		if !ok {
			continue
		}
		if m["name"] == "http" && m["href"] == wantURL {
			found = true
		}
	}
	if !found {
		t.Errorf("HTTP clone link %q not found in links %v", wantURL, links)
	}
}

func TestBuildWebhook_WrongSecret(t *testing.T) {
	p := bitbucketserver.Provider{}
	event := core.PushEvent{
		Host: "bb.example.com", Owner: "acme", Repo: "w", Branch: "main", After: testAfter,
	}

	headers, body, err := p.BuildWebhook(event, "correct-secret")
	if err != nil {
		t.Fatal(err)
	}
	headers.Set("X-Hub-Signature", "sha256=0000000000000000000000000000000000000000000000000000000000000000")

	hook, _ := bswebhooks.New(bswebhooks.Options.Secret("correct-secret"))
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	_, err = hook.Parse(req, bswebhooks.RepositoryReferenceChangedEvent)
	if err == nil {
		t.Fatal("expected HMAC-mismatch error, got nil")
	}
}

func TestBuildWebhook_NoSecret(t *testing.T) {
	p := bitbucketserver.Provider{}
	event := core.PushEvent{
		Host: "bb.example.com", Owner: "acme", Repo: "w", Branch: "main", After: testAfter,
	}
	headers, body, err := p.BuildWebhook(event, "")
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("X-Hub-Signature") != "" {
		t.Fatal("X-Hub-Signature must be absent when secret is empty")
	}
	roundTrip(t, headers, body, "") // must not error
}

func TestRepoURL(t *testing.T) {
	p := bitbucketserver.Provider{}
	got := p.RepoURL("bb.example.com", "acme", "widget")
	want := "http://bb.example.com/acme/widget.git"
	if got != want {
		t.Fatalf("RepoURL: want %q, got %q", want, got)
	}
}
