package main

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/brigadecore/brigade/sdk/v2/restmachinery"
	clientRM "github.com/brigadecore/brigade/sdk/v2/restmachinery"
	"github.com/stretchr/testify/require"
)

func TestAPIClientConfig(t *testing.T) {
	testCases := []struct {
		name       string
		setup      func()
		assertions func(
			address string,
			token string,
			opts clientRM.APIClientOptions,
			err error,
		)
	}{
		{
			name: "API_ADDRESS not set",
			assertions: func(
				_ string,
				_ string,
				_ clientRM.APIClientOptions,
				err error,
			) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "value not found for")
				require.Contains(t, err.Error(), "API_ADDRESS")
			},
		},
		{
			name: "API_TOKEN not set",
			setup: func() {
				os.Setenv("API_ADDRESS", "foo")
			},
			assertions: func(
				_ string,
				_ string,
				_ restmachinery.APIClientOptions,
				err error,
			) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "value not found for")
				require.Contains(t, err.Error(), "API_TOKEN")
			},
		},
		{
			name: "success",
			setup: func() {
				os.Setenv("API_TOKEN", "bar")
				os.Setenv("API_IGNORE_CERT_WARNINGS", "true")
			},
			assertions: func(
				address string,
				token string,
				opts restmachinery.APIClientOptions,
				err error,
			) {
				require.NoError(t, err)
				require.Equal(t, "foo", address)
				require.Equal(t, "bar", token)
				require.True(t, opts.AllowInsecureConnections)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.setup != nil {
				testCase.setup()
			}
			address, token, opts, err := apiClientConfig()
			testCase.assertions(address, token, opts, err)
		})
	}
}

func TestGetMonitorConfig(t *testing.T) {
	tmpFile, err := ioutil.TempFile(os.TempDir(), "")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	_, err = tmpFile.Write([]byte("foo"))
	require.NoError(t, err)
	testCases := []struct {
		name       string
		setup      func()
		assertions func(cfg monitorConfig, err error)
	}{
		{
			name: "GITHUB_APP_ID not set",
			assertions: func(_ monitorConfig, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "value not found for")
				require.Contains(t, err.Error(), "GITHUB_APP_ID")
			},
		},
		{
			name: "github API key missing",
			setup: func() {
				os.Setenv("GITHUB_APP_ID", "12345")
			},
			assertions: func(cfg monitorConfig, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "no such file or directory")
			},
		},
		{
			name: "errors parsing LIST_EVENTS_INTERVAL",
			setup: func() {
				os.Setenv("GITHUB_API_KEY_PATH", tmpFile.Name())
				os.Setenv("LIST_EVENTS_INTERVAL", "foo")
			},
			assertions: func(cfg monitorConfig, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "LIST_EVENTS_INTERVAL")
				require.Contains(t, err.Error(), "was not parsable as a duration")
			},
		},
		{
			name: "errors parsing EVENT_FOLLOW_UP_INTERVAL",
			setup: func() {
				os.Setenv("LIST_EVENTS_INTERVAL", "1m")
				os.Setenv("EVENT_FOLLOW_UP_INTERVAL", "foo")
			},
			assertions: func(cfg monitorConfig, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "EVENT_FOLLOW_UP_INTERVAL")
				require.Contains(t, err.Error(), "was not parsable as a duration")
			},
		},
		{
			name: "success",
			setup: func() {
				os.Setenv("EVENT_FOLLOW_UP_INTERVAL", "1m")
			},
			assertions: func(cfg monitorConfig, err error) {
				require.NoError(t, err)
				require.Equal(t, 12345, cfg.githubAppID)
				require.Equal(t, []byte("foo"), cfg.githubAPIKey)
				require.Equal(t, time.Minute, cfg.listEventsInterval)
				require.Equal(t, time.Minute, cfg.eventFollowUpInterval)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.setup != nil {
				testCase.setup()
			}
			cfg, err := getMonitorConfig()
			testCase.assertions(cfg, err)
		})
	}
}
