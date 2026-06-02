package gitserver

import (
	"fmt"
	"net/http"

	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gogitserver "github.com/go-git/go-git/v5/plumbing/transport/server"
)

// storerLoader implements gogitserver.Loader, always returning the same storer.
type storerLoader struct{ s storer.Storer }

func (l storerLoader) Load(_ *transport.Endpoint) (storer.Storer, error) { return l.s, nil }

func newSession(s storer.Storer) (transport.UploadPackSession, error) {
	ep, err := transport.NewEndpoint("git://localhost/repo.git")
	if err != nil {
		return nil, err
	}
	return gogitserver.NewServer(storerLoader{s}).NewUploadPackSession(ep, nil)
}

// infoRefs handles GET /{owner}/{repo}.git/info/refs?service=git-upload-pack.
// It writes the smart-HTTP service announcement followed by the ref advertisement.
func infoRefs(w http.ResponseWriter, r *http.Request, s storer.Storer) {
	if r.URL.Query().Get("service") != "git-upload-pack" {
		http.Error(w, "only git-upload-pack is supported", http.StatusForbidden)
		return
	}

	sess, err := newSession(s)
	if err != nil {
		http.Error(w, fmt.Sprintf("create session: %v", err), http.StatusInternalServerError)
		return
	}
	defer sess.Close()

	refs, err := sess.AdvertisedReferencesContext(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("advertise refs: %v", err), http.StatusInternalServerError)
		return
	}

	// Advertise shallow so go-git clients with Depth>0 can proceed. The server
	// does not enforce depth; it always sends a full pack with an empty
	// ShallowUpdate (0000), which the client treats as a full clone.
	_ = refs.Capabilities.Set(capability.Shallow)

	w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
	w.Header().Set("Cache-Control", "no-cache")

	// Smart-HTTP framing: pkt-line service header + flush, then the ref advertisement.
	enc := pktline.NewEncoder(w)
	if err := enc.Encodef("# service=git-upload-pack\n"); err != nil {
		return
	}
	if err := enc.Flush(); err != nil {
		return
	}
	_ = refs.Encode(w)
}

// uploadPack handles POST /{owner}/{repo}.git/git-upload-pack.
// It drives a full upload-pack session: init caps, decode client request, stream packfile.
func uploadPack(w http.ResponseWriter, r *http.Request, s storer.Storer) {
	sess, err := newSession(s)
	if err != nil {
		http.Error(w, fmt.Sprintf("create session: %v", err), http.StatusInternalServerError)
		return
	}
	defer sess.Close()

	// AdvertisedReferencesContext must be called to initialise the session's
	// capability state before UploadPack can validate the request's caps.
	advertised, err := sess.AdvertisedReferencesContext(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("init session: %v", err), http.StatusInternalServerError)
		return
	}

	// Must match what infoRefs advertised so checkSupportedCapabilities passes
	// when the client sends the shallow capability alongside a depth request.
	_ = advertised.Capabilities.Set(capability.Shallow)

	req := packp.NewUploadPackRequest()
	if err := req.Decode(r.Body); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	resp, err := sess.UploadPack(r.Context(), req)
	if err != nil {
		http.Error(w, fmt.Sprintf("upload pack: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Close()

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")
	_ = resp.Encode(w)
}
