package bitbucketserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	bspayload "github.com/go-playground/webhooks/v6/bitbucket-server"

	"github.com/rancher/gitsim/pkg/core"
)

func buildWebhook(event core.PushEvent, secret string) (http.Header, []byte, error) {
	var pl bspayload.RepositoryReferenceChangedPayload
	pl.EventKey = bspayload.RepositoryReferenceChangedEvent

	refID := "refs/heads/" + event.Branch
	if event.Tag != "" {
		refID = "refs/tags/" + event.Tag
	}

	pl.Changes = []bspayload.RepositoryChange{
		{
			Reference:   bspayload.RepositoryReference{ID: refID, DisplayID: event.Branch},
			ReferenceID: refID,
			FromHash:    event.Before,
			ToHash:      event.After,
			Type:        "UPDATE",
		},
	}

	// Populate the clone links so Fleet can correlate the payload with a GitRepo.
	// Fleet reads repository.links.clone[].href where name=="http".
	httpURL := Provider{}.RepoURL(event.Host, event.Owner, event.Repo)
	pl.Repository.Links = map[string]interface{}{
		"clone": []interface{}{
			map[string]interface{}{"name": "http", "href": httpURL},
			map[string]interface{}{"name": "ssh", "href": "ssh://git@" + event.Host + ":7999/" + event.Owner + "/" + event.Repo + ".git"},
		},
	}

	body, err := json.Marshal(pl)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal bitbucketserver payload: %w", err)
	}

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("X-Event-Key", string(bspayload.RepositoryReferenceChangedEvent))
	// Do NOT set X-Hook-UUID — Bitbucket Server must not include it (Fleet detection order).

	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		h.Set("X-Hub-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	return h, body, nil
}
