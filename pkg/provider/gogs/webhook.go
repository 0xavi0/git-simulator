package gogs

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	gogsclient "github.com/gogits/go-gogs-client"

	"github.com/rancher/gitsim/pkg/core"
)

func buildWebhook(event core.PushEvent, secret string) (http.Header, []byte, error) {
	repo := &gogsclient.Repository{
		Name:    event.Repo,
		HTMLURL: Provider{}.RepoURL(event.Host, event.Owner, event.Repo),
	}

	ref := "refs/heads/" + event.Branch
	if event.Tag != "" {
		ref = "refs/tags/" + event.Tag
	}

	pl := &gogsclient.PushPayload{
		Ref:    ref,
		Before: event.Before,
		After:  event.After,
		Repo:   repo,
		Pusher: &gogsclient.User{},
		Sender: &gogsclient.User{},
	}

	body, err := json.Marshal(pl)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal gogs payload: %w", err)
	}

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("X-Gogs-Event", "push")

	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		h.Set("X-Gogs-Signature", hex.EncodeToString(mac.Sum(nil)))
	}

	return h, body, nil
}
