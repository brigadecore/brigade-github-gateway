package webhooks

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strconv"

	libHTTP "github.com/brigadecore/brigade-foundations/http"
	ghlib "github.com/brigadecore/brigade-github-gateway/internal/github"
	"github.com/google/go-github/v33/github"
)

// SignatureVerificationFilterConfig encapsulates configuration for the
// signature verification based auth filter.
type SignatureVerificationFilterConfig struct {
	// GitHubApps is a map of GitHub App configurations indexed by App ID.
	GitHubApps map[int64]ghlib.App
}

// signatureVerificationFilter is a component that implements the http.Filter
// interface and can conditionally allow or disallow a request based on the
// ability to verify the signature of the inbound request.
type signatureVerificationFilter struct {
	config SignatureVerificationFilterConfig
}

// NewSignatureVerificationFilter returns a component that implements the
// http.Filter interface and can conditionally allow or disallow a request based
// on the ability to verify the signature of the inbound request.
func NewSignatureVerificationFilter(
	config SignatureVerificationFilterConfig,
) libHTTP.Filter {
	return &signatureVerificationFilter{
		config: config,
	}
}

func (s *signatureVerificationFilter) Decorate(
	handle http.HandlerFunc,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If there is no app ID, fail right away.
		appIDStr := r.Header.Get("X-GitHub-Hook-Installation-Target-ID")
		if appIDStr == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		appID, err := strconv.ParseInt(appIDStr, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		// If there is no request body, fail right away or else we'll be staring
		// down the barrel of a nil pointer dereference.
		if r.Body == nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		// If we encounter an error reading the request body, we're just going to
		// roll with it. The empty request body will naturally make the signature
		// verification algorithm fail.
		bodyBytes, _ := ioutil.ReadAll(r.Body) // nolint: errcheck
		r.Body.Close()                         // nolint: errcheck
		// Replace the request body because the original read was destructive!
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))

		if err := github.ValidateSignature(
			r.Header.Get("X-Hub-Signature"),
			bodyBytes,
			// Don't worry about the case where no app is found. Things will fail
			// naturally.
			[]byte(s.config.GitHubApps[appID].SharedSecret),
		); err != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		// If we get this far, everything checks out. Handle the request.
		handle(w, r)
	}
}
