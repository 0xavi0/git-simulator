package azuredevops_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	adwebhooks "github.com/go-playground/webhooks/v6/azuredevops"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider/azuredevops"
)

const (
	testSecret = "ado-password"
	testBefore = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testAfter  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func roundTrip(t *testing.T, headers http.Header, body []byte, secret string) adwebhooks.GitPushEvent {
	t.Helper()
	var opts []adwebhooks.Option
	if secret != "" {
		opts = append(opts, adwebhooks.Options.BasicAuth("", secret))
	}
	hook, err := adwebhooks.New(opts...)
	if err != nil {
		t.Fatalf("adwebhooks.New: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	payload, err := hook.Parse(req, adwebhooks.GitPushEventType)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	pl, ok := payload.(adwebhooks.GitPushEvent)
	if !ok {
		t.Fatalf("expected GitPushEvent, got %T", payload)
	}
	return pl
}

func TestBuildWebhook_Push_RoundTrip(t *testing.T) {
	p := azuredevops.Provider{}
	event := core.PushEvent{
		Host:   "dev.azure.com",
		Owner:  "myorg",
		Repo:   "myrepo",
		Branch: "main",
		Before: testBefore,
		After:  testAfter,
	}

	headers, body, err := p.BuildWebhook(event, testSecret)
	if err != nil {
		t.Fatalf("BuildWebhook: %v", err)
	}
	if headers.Get("X-Vss-Activityid") == "" {
		t.Fatal("X-Vss-Activityid must be present")
	}
	if headers.Get("X-Vss-Subscriptionid") == "" {
		t.Fatal("X-Vss-Subscriptionid must be present")
	}
	if headers.Get("Authorization") == "" {
		t.Fatal("Authorization must be present when secret is non-empty")
	}

	pl := roundTrip(t, headers, body, testSecret)

	if len(pl.Resource.RefUpdates) == 0 {
		t.Fatal("RefUpdates is empty")
	}
	if pl.Resource.RefUpdates[0].NewObjectID != testAfter {
		t.Errorf("NewObjectId: want %q, got %q", testAfter, pl.Resource.RefUpdates[0].NewObjectID)
	}
	if pl.Resource.RefUpdates[0].Name != "refs/heads/main" {
		t.Errorf("RefUpdate.Name: want refs/heads/main, got %q", pl.Resource.RefUpdates[0].Name)
	}
	wantURL := p.RepoURL("dev.azure.com", "myorg", "myrepo")
	if pl.Resource.Repository.RemoteURL != wantURL {
		t.Errorf("Repository.RemoteURL: want %q, got %q", wantURL, pl.Resource.Repository.RemoteURL)
	}
}

func TestBuildWebhook_WrongPassword(t *testing.T) {
	p := azuredevops.Provider{}
	event := core.PushEvent{
		Host: "dev.azure.com", Owner: "org", Repo: "r", Branch: "main", After: testAfter,
	}

	headers, body, err := p.BuildWebhook(event, "correct-pass")
	if err != nil {
		t.Fatal(err)
	}
	// Tamper with auth header.
	headers.Set("Authorization", "Basic d3Jvbmc=") // base64 ":wrong"

	hook, _ := adwebhooks.New(adwebhooks.Options.BasicAuth("", "correct-pass"))
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	_, err = hook.Parse(req, adwebhooks.GitPushEventType)
	if err == nil {
		t.Fatal("expected basic-auth-mismatch error, got nil")
	}
}

func TestBuildWebhook_NoAuth(t *testing.T) {
	p := azuredevops.Provider{}
	event := core.PushEvent{
		Host: "dev.azure.com", Owner: "org", Repo: "r", Branch: "main", After: testAfter,
	}
	headers, body, err := p.BuildWebhook(event, "")
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("Authorization") != "" {
		t.Fatal("Authorization must be absent when secret is empty")
	}
	roundTrip(t, headers, body, "") // no auth configured on receiver → must pass
}

func TestRepoURL(t *testing.T) {
	p := azuredevops.Provider{}
	got := p.RepoURL("dev.azure.com", "myorg", "myrepo")
	want := "https://dev.azure.com/myorg/_git/myrepo"
	if got != want {
		t.Fatalf("RepoURL: want %q, got %q", want, got)
	}
}
