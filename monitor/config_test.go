package main

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/brigadecore/brigade/sdk/v2/restmachinery"
	"github.com/stretchr/testify/require"
)

func TestAPIClientConfig(t *testing.T) {
	testCases := []struct {
		name       string
		setup      func()
		assertions func(
			address string,
			token string,
			opts restmachinery.APIClientOptions,
			err error,
		)
	}{
		{
			name: "API_ADDRESS not set",
			assertions: func(
				_ string,
				_ string,
				_ restmachinery.APIClientOptions,
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
				t.Setenv("API_ADDRESS", "foo")
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
				t.Setenv("API_TOKEN", "bar")
				t.Setenv("API_IGNORE_CERT_WARNINGS", "true")
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
			name: "GITHUB_APPS_PATH not set",
			assertions: func(_ monitorConfig, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "value not found for")
				require.Contains(t, err.Error(), "GITHUB_APPS_PATH")
			},
		},
		{
			name: "GITHUB_APPS_PATH path does not exist",
			setup: func() {
				t.Setenv("GITHUB_APPS_PATH", "/completely/bogus/path")
			},
			assertions: func(_ monitorConfig, err error) {
				require.Error(t, err)
				require.Contains(
					t,
					err.Error(),
					"file /completely/bogus/path does not exist",
				)
			},
		},
		{
			name: "GITHUB_APPS_PATH does not contain valid json",
			setup: func() {
				appsFile, err := ioutil.TempFile("", "apps.json")
				require.NoError(t, err)
				defer appsFile.Close()
				_, err = appsFile.Write([]byte("this is not json"))
				require.NoError(t, err)
				t.Setenv("GITHUB_APPS_PATH", appsFile.Name())
			},
			assertions: func(_ monitorConfig, err error) {
				require.Error(t, err)
				require.Contains(
					t, err.Error(), "invalid character",
				)
			},
		},
		{
			name: "errors parsing LIST_EVENTS_INTERVAL",
			setup: func() {
				appsFile, err := ioutil.TempFile("", "apps.json")
				require.NoError(t, err)
				defer appsFile.Close()
				_, err =
					appsFile.Write([]byte(`[{"appID":42,"apiKey":"foobar"}]`))
				require.NoError(t, err)
				t.Setenv("GITHUB_APPS_PATH", appsFile.Name())
				t.Setenv("LIST_EVENTS_INTERVAL", "foo")
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
				appsFile, err := ioutil.TempFile("", "apps.json")
				require.NoError(t, err)
				defer appsFile.Close()
				_, err =
					appsFile.Write([]byte(`[{"appID":42,"apiKey":"foobar"}]`))
				require.NoError(t, err)
				t.Setenv("GITHUB_APPS_PATH", appsFile.Name())
				t.Setenv("LIST_EVENTS_INTERVAL", "1m")
				t.Setenv("EVENT_FOLLOW_UP_INTERVAL", "foo")
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
				appsFile, err := ioutil.TempFile("", "apps.json")
				require.NoError(t, err)
				defer appsFile.Close()
				_, err =
					appsFile.Write([]byte(`[{"appID":42,"apiKey":"foobar"}]`))
				require.NoError(t, err)
				t.Setenv("GITHUB_APPS_PATH", appsFile.Name())
				t.Setenv("EVENT_FOLLOW_UP_INTERVAL", "1m")
			},
			assertions: func(cfg monitorConfig, err error) {
				require.NoError(t, err)
				require.Len(t, cfg.gitHubApps, 1)
				require.Equal(t, int64(42), cfg.gitHubApps[42].AppID)
				require.Equal(t, "foobar", cfg.gitHubApps[42].APIKey)
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
