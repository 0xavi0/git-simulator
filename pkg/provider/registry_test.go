package provider_test

import (
	"testing"

	"github.com/rancher/gitsim/pkg/provider"
	// Blank import triggers init(), registering the GitHub provider.
	_ "github.com/rancher/gitsim/pkg/provider/github"
)

func TestGet_GitHub(t *testing.T) {
	p, err := provider.Get("github")
	if err != nil {
		t.Fatalf("Get(github): %v", err)
	}
	if p == nil {
		t.Fatal("Get(github) returned nil provider")
	}
	if p.Name() != "github" {
		t.Fatalf("Name(): want github, got %s", p.Name())
	}
}

func TestGet_Unknown(t *testing.T) {
	_, err := provider.Get("does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}
