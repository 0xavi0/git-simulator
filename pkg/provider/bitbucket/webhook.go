package bitbucket

import (
	"encoding/json"
	"fmt"
	"net/http"

	bbpayload "github.com/go-playground/webhooks/v6/bitbucket"

	"github.com/rancher/gitsim/pkg/core"
)

func buildWebhook(event core.PushEvent, uuid string) (http.Header, []byte, error) {
	var pl bbpayload.RepoPushPayload

	// Set the repo URL in Repository.Links.HTML.Href — the field Fleet reads.
	pl.Repository.Links.HTML.Href = Provider{}.RepoURL(event.Host, event.Owner, event.Repo)

	// Build the push change entry.
	var change struct {
		New struct {
			Type   string `json:"type"`
			Name   string `json:"name"`
			Target struct {
				Type string `json:"type"`
				Hash string `json:"hash"`
			} `json:"target"`
		} `json:"new"`
	}

	if event.Tag != "" {
		change.New.Type = "tag"
		change.New.Name = event.Tag
	} else {
		change.New.Type = "branch"
		change.New.Name = event.Branch
	}
	change.New.Target.Type = "commit"
	change.New.Target.Hash = event.After

	// The RepoPushPayload.Push.Changes field is an anonymous struct slice whose
	// New subfield carries the target hash. Marshal via a compatible map so we
	// don't have to replicate the entire nested anonymous-struct chain.
	raw := map[string]interface{}{
		"actor":      map[string]interface{}{},
		"repository": pl.Repository,
		"push": map[string]interface{}{
			"changes": []interface{}{change},
		},
	}

	body, err := json.Marshal(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal bitbucket payload: %w", err)
	}

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("X-Event-Key", "repo:push")
	if uuid != "" {
		h.Set("X-Hook-UUID", uuid)
	}

	return h, body, nil
}
