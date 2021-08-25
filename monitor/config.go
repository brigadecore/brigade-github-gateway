package main

import (
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/brigadecore/brigade-foundations/file"
	"github.com/brigadecore/brigade-foundations/os"
	"github.com/brigadecore/brigade-github-gateway/internal/github"
	clientRM "github.com/brigadecore/brigade/sdk/v2/restmachinery"
	"github.com/pkg/errors"
)

// apiClientConfig populates the Brigade SDK's APIClientOptions from
// environment variables.
func apiClientConfig() (string, string, clientRM.APIClientOptions, error) {
	opts := clientRM.APIClientOptions{}
	address, err := os.GetRequiredEnvVar("API_ADDRESS")
	if err != nil {
		return address, "", opts, err
	}
	token, err := os.GetRequiredEnvVar("API_TOKEN")
	if err != nil {
		return address, token, opts, err
	}
	opts.AllowInsecureConnections, err =
		os.GetBoolFromEnvVar("API_IGNORE_CERT_WARNINGS", false)
	return address, token, opts, err
}

// webhookServiceConfig populates configuration for the monitor from environment
// variables.
func getMonitorConfig() (monitorConfig, error) {
	config := monitorConfig{
		healthcheckInterval: 30 * time.Second,
		gitHubApps:          map[int64]github.App{},
	}
	githubAppsPath, err := os.GetRequiredEnvVar("GITHUB_APPS_PATH")
	if err != nil {
		return config, err
	}
	var exists bool
	if exists, err = file.Exists(githubAppsPath); err != nil {
		return config, err
	}
	if !exists {
		return config, errors.Errorf("file %s does not exist", githubAppsPath)
	}
	githubAppsBytes, err := ioutil.ReadFile(githubAppsPath)
	if err != nil {
		return config, err
	}
	githubApps := []github.App{}
	if err = json.Unmarshal(githubAppsBytes, &githubApps); err != nil {
		return config, err
	}
	for _, githubApp := range githubApps {
		config.gitHubApps[githubApp.AppID] = githubApp
	}
	config.listEventsInterval, err =
		os.GetDurationFromEnvVar("LIST_EVENTS_INTERVAL", 30*time.Second)
	if err != nil {
		return config, err
	}
	config.eventFollowUpInterval, err =
		os.GetDurationFromEnvVar("EVENT_FOLLOW_UP_INTERVAL", 30*time.Second)
	return config, err
}
