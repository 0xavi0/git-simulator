// Package validation_test consolidates cross-provider round-trip guarantees.
// Each provider builds a push webhook; the matching go-playground/webhooks parser
// (the same library Fleet uses) must accept it with the correct secret and reject
// it after the auth header is tampered.
package validation_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adwebhooks "github.com/go-playground/webhooks/v6/azuredevops"
	bbwebhooks "github.com/go-playground/webhooks/v6/bitbucket"
	bswebhooks "github.com/go-playground/webhooks/v6/bitbucket-server"
	ghwebhooks "github.com/go-playground/webhooks/v6/github"
	glwebhooks "github.com/go-playground/webhooks/v6/gitlab"
	gogswebhooks "github.com/go-playground/webhooks/v6/gogs"
	gogsclient "github.com/gogits/go-gogs-client"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider/azuredevops"
	"github.com/rancher/gitsim/pkg/provider/bitbucket"
	"github.com/rancher/gitsim/pkg/provider/bitbucketserver"
	"github.com/rancher/gitsim/pkg/provider/github"
	"github.com/rancher/gitsim/pkg/provider/gitlab"
	"github.com/rancher/gitsim/pkg/provider/gogs"
)

const (
	matrixBefore = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	matrixAfter  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// providerCase is a single row in the webhook matrix.
type providerCase struct {
	name   string
	prov   core.Provider
	event  core.PushEvent
	secret string
	// parse creates a receiver configured with secret and parses the request.
	// Returns the "after" SHA extracted from the payload.
	parse func(headers http.Header, body []byte, secret string) (afterSHA string, err error)
	// tamper corrupts the auth credential so that parse returns an error.
	tamper func(h http.Header)
}

var cases = []providerCase{
	{
		name:   "github",
		prov:   github.Provider{},
		event:  core.PushEvent{Host: "github.com", Owner: "acme", Repo: "app", Branch: "main", Before: matrixBefore, After: matrixAfter},
		secret: "gh-secret",
		parse: func(headers http.Header, body []byte, secret string) (string, error) {
			var opts []ghwebhooks.Option
			if secret != "" {
				opts = append(opts, ghwebhooks.Options.Secret(secret))
			}
			hook, err := ghwebhooks.New(opts...)
			if err != nil {
				return "", err
			}
			pl, err := hook.Parse(fakeReq(headers, body), ghwebhooks.PushEvent)
			if err != nil {
				return "", err
			}
			return pl.(ghwebhooks.PushPayload).After, nil
		},
		tamper: func(h http.Header) {
			h.Set("X-Hub-Signature-256", "sha256=0000000000000000000000000000000000000000000000000000000000000000")
		},
	},
	{
		name:   "gitlab",
		prov:   gitlab.Provider{},
		event:  core.PushEvent{Host: "gitlab.com", Owner: "acme", Repo: "app", Branch: "main", Before: matrixBefore, After: matrixAfter},
		secret: "gl-secret",
		parse: func(headers http.Header, body []byte, secret string) (string, error) {
			var opts []glwebhooks.Option
			if secret != "" {
				opts = append(opts, glwebhooks.Options.Secret(secret))
			}
			hook, err := glwebhooks.New(opts...)
			if err != nil {
				return "", err
			}
			pl, err := hook.Parse(fakeReq(headers, body), glwebhooks.PushEvents)
			if err != nil {
				return "", err
			}
			return pl.(glwebhooks.PushEventPayload).CheckoutSHA, nil
		},
		tamper: func(h http.Header) { h.Set("X-Gitlab-Token", "wrong-token") },
	},
	{
		name:   "bitbucket",
		prov:   bitbucket.Provider{},
		event:  core.PushEvent{Host: "bitbucket.org", Owner: "acme", Repo: "app", Branch: "main", Before: matrixBefore, After: matrixAfter},
		secret: "550e8400-e29b-41d4-a716-446655440000",
		parse: func(headers http.Header, body []byte, secret string) (string, error) {
			var opts []bbwebhooks.Option
			if secret != "" {
				opts = append(opts, bbwebhooks.Options.UUID(secret))
			}
			hook, err := bbwebhooks.New(opts...)
			if err != nil {
				return "", err
			}
			pl, err := hook.Parse(fakeReq(headers, body), bbwebhooks.RepoPushEvent)
			if err != nil {
				return "", err
			}
			changes := pl.(bbwebhooks.RepoPushPayload).Push.Changes
			if len(changes) == 0 {
				return "", fmt.Errorf("no push changes in Bitbucket payload")
			}
			return changes[0].New.Target.Hash, nil
		},
		tamper: func(h http.Header) { h.Set("X-Hook-UUID", "different-uuid") },
	},
	{
		name:   "bitbucketserver",
		prov:   bitbucketserver.Provider{},
		event:  core.PushEvent{Host: "bb.example.com", Owner: "acme", Repo: "app", Branch: "main", Before: matrixBefore, After: matrixAfter},
		secret: "bbs-secret",
		parse: func(headers http.Header, body []byte, secret string) (string, error) {
			var opts []bswebhooks.Option
			if secret != "" {
				opts = append(opts, bswebhooks.Options.Secret(secret))
			}
			hook, err := bswebhooks.New(opts...)
			if err != nil {
				return "", err
			}
			pl, err := hook.Parse(fakeReq(headers, body), bswebhooks.RepositoryReferenceChangedEvent)
			if err != nil {
				return "", err
			}
			changes := pl.(bswebhooks.RepositoryReferenceChangedPayload).Changes
			if len(changes) == 0 {
				return "", fmt.Errorf("no changes in Bitbucket Server payload")
			}
			return changes[0].ToHash, nil
		},
		tamper: func(h http.Header) {
			h.Set("X-Hub-Signature", "sha256=0000000000000000000000000000000000000000000000000000000000000000")
		},
	},
	{
		name:   "gogs",
		prov:   gogs.Provider{},
		event:  core.PushEvent{Host: "gogs.example.com", Owner: "acme", Repo: "app", Branch: "main", Before: matrixBefore, After: matrixAfter},
		secret: "gogs-secret",
		parse: func(headers http.Header, body []byte, secret string) (string, error) {
			var opts []gogswebhooks.Option
			if secret != "" {
				opts = append(opts, gogswebhooks.Options.Secret(secret))
			}
			hook, err := gogswebhooks.New(opts...)
			if err != nil {
				return "", err
			}
			pl, err := hook.Parse(fakeReq(headers, body), gogswebhooks.PushEvent)
			if err != nil {
				return "", err
			}
			return pl.(gogsclient.PushPayload).After, nil
		},
		tamper: func(h http.Header) {
			h.Set("X-Gogs-Signature", "0000000000000000000000000000000000000000000000000000000000000000")
		},
	},
	{
		name:   "azuredevops",
		prov:   azuredevops.Provider{},
		event:  core.PushEvent{Host: "dev.azure.com", Owner: "myorg", Repo: "myrepo", Branch: "main", Before: matrixBefore, After: matrixAfter},
		secret: "ado-password",
		parse: func(headers http.Header, body []byte, secret string) (string, error) {
			var opts []adwebhooks.Option
			if secret != "" {
				opts = append(opts, adwebhooks.Options.BasicAuth("", secret))
			}
			hook, err := adwebhooks.New(opts...)
			if err != nil {
				return "", err
			}
			pl, err := hook.Parse(fakeReq(headers, body), adwebhooks.GitPushEventType)
			if err != nil {
				return "", err
			}
			updates := pl.(adwebhooks.GitPushEvent).Resource.RefUpdates
			if len(updates) == 0 {
				return "", fmt.Errorf("no ref updates in Azure DevOps payload")
			}
			return updates[0].NewObjectID, nil
		},
		tamper: func(h http.Header) {
			h.Set("Authorization", "Basic d3Jvbmc=") // base64(":wrong")
		},
	},
}

// TestWebhookMatrix_CorrectAuth verifies that each provider's BuildWebhook output
// parses cleanly with the correct secret and carries the expected "after" SHA.
func TestWebhookMatrix_CorrectAuth(t *testing.T) {
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			headers, body, err := tc.prov.BuildWebhook(tc.event, tc.secret)
			if err != nil {
				t.Fatalf("BuildWebhook: %v", err)
			}
			got, err := tc.parse(headers, body, tc.secret)
			if err != nil {
				t.Fatalf("parse with correct auth: %v", err)
			}
			if got != matrixAfter {
				t.Errorf("after SHA: got %q want %q", got, matrixAfter)
			}
		})
	}
}

// TestWebhookMatrix_WrongAuth verifies that each provider's payload is rejected
// when the auth header is tampered (simulating a wrong secret or UUID).
func TestWebhookMatrix_WrongAuth(t *testing.T) {
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			headers, body, err := tc.prov.BuildWebhook(tc.event, tc.secret)
			if err != nil {
				t.Fatalf("BuildWebhook: %v", err)
			}
			tc.tamper(headers)
			_, err = tc.parse(headers, body, tc.secret)
			if err == nil {
				t.Fatal("expected parse error with tampered auth, got nil")
			}
		})
	}
}

func fakeReq(headers http.Header, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	return req
}
