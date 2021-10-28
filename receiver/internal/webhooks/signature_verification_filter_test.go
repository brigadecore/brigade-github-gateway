package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	ghlib "github.com/brigadecore/brigade-github-gateway/internal/github"
	"github.com/stretchr/testify/require"
)

func TestNewSignatureVerificationFilter(t *testing.T) {
	const testAppID int64 = 42
	testSecret := []byte("foobar")
	testConfig := SignatureVerificationFilterConfig{
		GitHubApps: map[int64]ghlib.App{
			testAppID: {
				AppID:        testAppID,
				SharedSecret: string(testSecret),
			},
		},
	}
	filter := // nolint: forcetypeassert
		NewSignatureVerificationFilter(testConfig).(*signatureVerificationFilter)
	require.Equal(t, testConfig, filter.config)
}

func TestSignatureVerificationFilter(t *testing.T) {
	const testAppID int64 = 42
	testSecret := []byte("foobar")
	testFilter := &signatureVerificationFilter{
		config: SignatureVerificationFilterConfig{
			GitHubApps: map[int64]ghlib.App{
				testAppID: {
					AppID:        testAppID,
					SharedSecret: string(testSecret),
				},
			},
		},
	}
	testCases := []struct {
		name       string
		setup      func() *http.Request
		assertions func(handlerCalled bool, rr *httptest.ResponseRecorder)
	}{
		{
			name: "app ID header absent",
			setup: func() *http.Request {
				bodyBytes := []byte("mr body")
				req, err :=
					http.NewRequest(http.MethodPost, "/", bytes.NewBuffer(bodyBytes))
				require.NoError(t, err)
				return req
			},
			assertions: func(handlerCalled bool, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusForbidden, rr.Code)
				require.False(t, handlerCalled)
			},
		},
		{
			name: "app ID header not parseable as int",
			setup: func() *http.Request {
				bodyBytes := []byte("mr body")
				req, err :=
					http.NewRequest(http.MethodPost, "/", bytes.NewBuffer(bodyBytes))
				require.NoError(t, err)
				// This is not parseable as an int
				req.Header.Add("X-GitHub-Hook-Installation-Target-ID", "foo")
				return req
			},
			assertions: func(handlerCalled bool, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusForbidden, rr.Code)
				require.False(t, handlerCalled)
			},
		},
		{
			name: "signature cannot be verified",
			setup: func() *http.Request {
				bodyBytes := []byte("mr body")
				req, err :=
					http.NewRequest(http.MethodPost, "/", bytes.NewBuffer(bodyBytes))
				require.NoError(t, err)
				// Add app ID to the request
				req.Header.Add(
					"X-GitHub-Hook-Installation-Target-ID",
					strconv.Itoa(int(testAppID)),
				)
				// This is just a completely made up signature
				req.Header.Add("X-Hub-Signature", "johnhancock")
				return req
			},
			assertions: func(handlerCalled bool, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusForbidden, rr.Code)
				require.False(t, handlerCalled)
			},
		},
		{
			name: "signature can be verified",
			setup: func() *http.Request {
				bodyBytes := []byte("mr body")
				req, err :=
					http.NewRequest(http.MethodPost, "/", bytes.NewBuffer(bodyBytes))
				require.NoError(t, err)
				// Compute the signature
				hasher := hmac.New(sha256.New, testSecret)
				_, err = hasher.Write(bodyBytes)
				require.NoError(t, err)
				// Add app ID to the request
				req.Header.Add(
					"X-GitHub-Hook-Installation-Target-ID",
					strconv.Itoa(int(testAppID)),
				)
				// Add the signature to the request
				req.Header.Add(
					"X-Hub-Signature",
					fmt.Sprintf("sha256=%x", hasher.Sum(nil)),
				)
				return req
			},
			assertions: func(handlerCalled bool, rr *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, rr.Code)
				require.True(t, handlerCalled)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := testCase.setup()
			handlerCalled := false
			testFilter.Decorate(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusOK)
			})(rr, req)
			testCase.assertions(handlerCalled, rr)
		})
	}
}
