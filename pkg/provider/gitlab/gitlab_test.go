package gitlab_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	glwebhooks "github.com/go-playground/webhooks/v6/gitlab"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider/gitlab"
)

const (
	testSecret = "gitlab-secret"
	testBefore = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testAfter  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func roundTrip(t *testing.T, headers http.Header, body []byte, secret string) glwebhooks.PushEventPayload {
	t.Helper()
	hook, err := glwebhooks.New(glwebhooks.Options.Secret(secret))
	if err != nil {
		t.Fatalf("glwebhooks.New: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	payload, err := hook.Parse(req, glwebhooks.PushEvents)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	pl, ok := payload.(glwebhooks.PushEventPayload)
	if !ok {
		t.Fatalf("expected PushEventPayload, got %T", payload)
	}
	return pl
}

func TestBuildWebhook_Push_RoundTrip(t *testing.T) {
	p := gitlab.Provider{}
	event := core.PushEvent{
		Host:   "gitlab.com",
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
	if got := headers.Get("X-Gitlab-Event"); got != "Push Hook" {
		t.Fatalf("X-Gitlab-Event: want Push Hook, got %q", got)
	}
	if got := headers.Get("X-Gitlab-Token"); got != testSecret {
		t.Fatalf("X-Gitlab-Token: want %q, got %q", testSecret, got)
	}

	pl := roundTrip(t, headers, body, testSecret)

	if pl.Ref != "refs/heads/main" {
		t.Errorf("Ref: want refs/heads/main, got %q", pl.Ref)
	}
	if pl.CheckoutSHA != testAfter {
		t.Errorf("CheckoutSHA: want %q, got %q", testAfter, pl.CheckoutSHA)
	}
	wantURL := p.RepoURL("gitlab.com", "acme", "widget")
	if pl.Project.WebURL != wantURL {
		t.Errorf("Project.WebURL: want %q, got %q", wantURL, pl.Project.WebURL)
	}
}

func TestBuildWebhook_Tag_RoundTrip(t *testing.T) {
	p := gitlab.Provider{}
	event := core.PushEvent{
		Host:  "gitlab.com",
		Owner: "acme",
		Repo:  "widget",
		Tag:   "v1.0.0",
		After: testAfter,
	}

	hook, _ := glwebhooks.New()
	headers, body, err := p.BuildWebhook(event, "")
	if err != nil {
		t.Fatalf("BuildWebhook: %v", err)
	}
	if got := headers.Get("X-Gitlab-Event"); got != "Tag Push Hook" {
		t.Fatalf("X-Gitlab-Event: want Tag Push Hook, got %q", got)
	}

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	payload, err := hook.Parse(req, glwebhooks.TagEvents)
	if err != nil {
		t.Fatalf("Parse tag: %v", err)
	}
	tag, ok := payload.(glwebhooks.TagEventPayload)
	if !ok {
		t.Fatalf("expected TagEventPayload, got %T", payload)
	}
	if tag.Ref != "refs/tags/v1.0.0" {
		t.Errorf("Ref: want refs/tags/v1.0.0, got %q", tag.Ref)
	}
	if tag.CheckoutSHA != testAfter {
		t.Errorf("CheckoutSHA: want %q, got %q", testAfter, tag.CheckoutSHA)
	}
}

func TestBuildWebhook_WrongToken(t *testing.T) {
	p := gitlab.Provider{}
	event := core.PushEvent{
		Host: "gitlab.com", Owner: "acme", Repo: "w", Branch: "main", After: testAfter,
	}

	headers, body, err := p.BuildWebhook(event, "correct")
	if err != nil {
		t.Fatal(err)
	}
	// Tamper with the token header.
	headers.Set("X-Gitlab-Token", "wrong-token")

	hook, _ := glwebhooks.New(glwebhooks.Options.Secret("correct"))
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	_, err = hook.Parse(req, glwebhooks.PushEvents)
	if err == nil {
		t.Fatal("expected token-mismatch error, got nil")
	}
}

func TestRepoURL(t *testing.T) {
	p := gitlab.Provider{}
	got := p.RepoURL("gitlab.com", "acme", "widget")
	want := "https://gitlab.com/acme/widget"
	if got != want {
		t.Fatalf("RepoURL: want %q, got %q", want, got)
	}
}
