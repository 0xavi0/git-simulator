package gogs_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	gogswebhooks "github.com/go-playground/webhooks/v6/gogs"
	gogsclient "github.com/gogits/go-gogs-client"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider/gogs"
)

const (
	testSecret = "gogs-secret"
	testAfter  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func roundTrip(t *testing.T, headers http.Header, body []byte, secret string) gogsclient.PushPayload {
	t.Helper()
	var opts []gogswebhooks.Option
	if secret != "" {
		opts = append(opts, gogswebhooks.Options.Secret(secret))
	}
	hook, err := gogswebhooks.New(opts...)
	if err != nil {
		t.Fatalf("gogswebhooks.New: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	payload, err := hook.Parse(req, gogswebhooks.PushEvent)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	pl, ok := payload.(gogsclient.PushPayload)
	if !ok {
		t.Fatalf("expected PushPayload, got %T", payload)
	}
	return pl
}

func TestBuildWebhook_Push_RoundTrip(t *testing.T) {
	p := gogs.Provider{}
	event := core.PushEvent{
		Host:   "gogs.example.com",
		Owner:  "acme",
		Repo:   "widget",
		Branch: "main",
		Before: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		After:  testAfter,
	}

	headers, body, err := p.BuildWebhook(event, testSecret)
	if err != nil {
		t.Fatalf("BuildWebhook: %v", err)
	}
	if got := headers.Get("X-Gogs-Event"); got != "push" {
		t.Fatalf("X-Gogs-Event: want push, got %q", got)
	}
	if headers.Get("X-Gogs-Signature") == "" {
		t.Fatal("X-Gogs-Signature must be present when secret is non-empty")
	}

	pl := roundTrip(t, headers, body, testSecret)

	if pl.After != testAfter {
		t.Errorf("After: want %q, got %q", testAfter, pl.After)
	}
	if pl.Ref != "refs/heads/main" {
		t.Errorf("Ref: want refs/heads/main, got %q", pl.Ref)
	}
	wantURL := p.RepoURL("gogs.example.com", "acme", "widget")
	if pl.Repo == nil || pl.Repo.HTMLURL != wantURL {
		t.Errorf("Repo.HTMLURL: want %q, got %v", wantURL, pl.Repo)
	}
}

func TestBuildWebhook_WrongSecret(t *testing.T) {
	p := gogs.Provider{}
	event := core.PushEvent{
		Host: "gogs.example.com", Owner: "acme", Repo: "w", Branch: "main", After: testAfter,
	}

	headers, body, err := p.BuildWebhook(event, "correct-secret")
	if err != nil {
		t.Fatal(err)
	}
	headers.Set("X-Gogs-Signature", "0000000000000000000000000000000000000000000000000000000000000000")

	hook, _ := gogswebhooks.New(gogswebhooks.Options.Secret("correct-secret"))
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	_, err = hook.Parse(req, gogswebhooks.PushEvent)
	if err == nil {
		t.Fatal("expected HMAC-mismatch error, got nil")
	}
}

func TestBuildWebhook_NoSecret(t *testing.T) {
	p := gogs.Provider{}
	event := core.PushEvent{
		Host: "gogs.example.com", Owner: "acme", Repo: "w", Branch: "main", After: testAfter,
	}
	headers, body, err := p.BuildWebhook(event, "")
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("X-Gogs-Signature") != "" {
		t.Fatal("X-Gogs-Signature must be absent when secret is empty")
	}
	roundTrip(t, headers, body, "") // must not error
}

func TestRepoURL(t *testing.T) {
	p := gogs.Provider{}
	got := p.RepoURL("gogs.example.com", "acme", "widget")
	want := "https://gogs.example.com/acme/widget"
	if got != want {
		t.Fatalf("RepoURL: want %q, got %q", want, got)
	}
}
