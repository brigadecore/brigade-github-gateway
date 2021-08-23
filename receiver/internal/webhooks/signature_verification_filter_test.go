package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSignatureVerificationFilter(t *testing.T) {
	testConfig := SignatureVerificationFilterConfig{
		SharedSecret: []byte("foobar"),
	}
	filter :=
		NewSignatureVerificationFilter(testConfig).(*signatureVerificationFilter)
	require.Equal(t, testConfig, filter.config)
}

func TestSignatureVerificationFilter(t *testing.T) {
	testSecret := []byte("foobar")
	testFilter := &signatureVerificationFilter{
		config: SignatureVerificationFilterConfig{
			SharedSecret: testSecret,
		},
	}
	testCases := []struct {
		name       string
		setup      func() *http.Request
		assertions func(handlerCalled bool, rr *httptest.ResponseRecorder)
	}{
		{
			name: "signature cannot be verified",
			setup: func() *http.Request {
				bodyBytes := []byte("mr body")
				req, err :=
					http.NewRequest(http.MethodPost, "/", bytes.NewBuffer(bodyBytes))
				require.NoError(t, err)
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
