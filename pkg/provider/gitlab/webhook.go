package gitlab

import (
	"encoding/json"
	"fmt"
	"net/http"

	glpayload "github.com/go-playground/webhooks/v6/gitlab"

	"github.com/rancher/gitsim/pkg/core"
)

func buildWebhook(event core.PushEvent, secret string) (http.Header, []byte, error) {
	var body []byte
	var err error
	var eventHeader string

	if event.Tag != "" {
		var pl glpayload.TagEventPayload
		pl.Ref = "refs/tags/" + event.Tag
		pl.CheckoutSHA = event.After
		pl.Project.WebURL = Provider{}.RepoURL(event.Host, event.Owner, event.Repo)
		body, err = json.Marshal(pl)
		eventHeader = "Tag Push Hook"
	} else {
		var pl glpayload.PushEventPayload
		pl.Ref = "refs/heads/" + event.Branch
		pl.CheckoutSHA = event.After
		pl.Project.WebURL = Provider{}.RepoURL(event.Host, event.Owner, event.Repo)
		body, err = json.Marshal(pl)
		eventHeader = "Push Hook"
	}
	if err != nil {
		return nil, nil, fmt.Errorf("marshal gitlab payload: %w", err)
	}

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("X-Gitlab-Event", eventHeader)
	if secret != "" {
		h.Set("X-Gitlab-Token", secret)
	}

	return h, body, nil
}
