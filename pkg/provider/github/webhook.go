package github

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	ghpayload "github.com/go-playground/webhooks/v6/github"

	"github.com/rancher/gitsim/pkg/core"
)

func buildPushWebhook(event core.PushEvent, secret string) (http.Header, []byte, error) {
	var pl ghpayload.PushPayload

	if event.Branch != "" {
		pl.Ref = "refs/heads/" + event.Branch
	} else if event.Tag != "" {
		pl.Ref = "refs/tags/" + event.Tag
	}
	pl.Before = event.Before
	pl.After = event.After
	pl.Repository.HTMLURL = Provider{}.RepoURL(event.Host, event.Owner, event.Repo)

	body, err := json.Marshal(pl)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal push payload: %w", err)
	}

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("X-GitHub-Event", "push")
	h.Set("X-GitHub-Delivery", newUUID())

	if secret != "" {
		h.Set("X-Hub-Signature-256", "sha256="+sign256(body, secret))
	}

	return h, body, nil
}

// BuildPingWebhook returns the headers and body for a GitHub ping event.
// Fleet responds to ping with "Webhook received successfully" without parsing
// the body, so only the header matters in practice.
func BuildPingWebhook() (http.Header, []byte, error) {
	var pl ghpayload.PingPayload
	body, err := json.Marshal(pl)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal ping payload: %w", err)
	}
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("X-GitHub-Event", "ping")
	h.Set("X-GitHub-Delivery", newUUID())
	return h, body, nil
}

// sign256 computes HMAC-SHA256 of body using secret and returns the hex digest.
func sign256(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// newUUID generates a random UUID v4 string.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
