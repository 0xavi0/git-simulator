package azuredevops

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	adpayload "github.com/go-playground/webhooks/v6/azuredevops"

	"github.com/rancher/gitsim/pkg/core"
)

func buildWebhook(event core.PushEvent, secret string) (http.Header, []byte, error) {
	refName := "refs/heads/" + event.Branch
	if event.Tag != "" {
		refName = "refs/tags/" + event.Tag
	}

	var pl adpayload.GitPushEvent
	pl.EventType = string(adpayload.GitPushEventType)
	pl.ID = newUUID()
	pl.PublisherID = "tfs"
	// BasicEvent.CreatedDate is a custom Date type that requires RFC3339Nano.
	pl.CreatedDate = time.Now().UTC().Format(time.RFC3339Nano)
	pl.Resource.Repository.RemoteURL = Provider{}.RepoURL(event.Host, event.Owner, event.Repo)
	pl.Resource.Repository.Name = event.Repo
	pl.Resource.RefUpdates = []adpayload.RefUpdate{
		{
			Name:        refName,
			NewObjectID: event.After,
			OldObjectID: event.Before,
		},
	}

	body, err := json.Marshal(pl)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal azuredevops payload: %w", err)
	}

	h := http.Header{}
	h.Set("Content-Type", "application/json")
	// Fleet identifies Azure DevOps by the presence of X-Vss-Activityid.
	h.Set("X-Vss-Activityid", newUUID())
	h.Set("X-Vss-Subscriptionid", newUUID())

	if secret != "" {
		// Basic auth: username="" password=secret
		credentials := base64.StdEncoding.EncodeToString([]byte(":" + secret))
		h.Set("Authorization", "Basic "+credentials)
	}

	return h, body, nil
}
