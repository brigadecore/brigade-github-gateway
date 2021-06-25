package main

import (
	"io/ioutil"
	"time"

	"github.com/brigadecore/brigade-github-gateway/internal/os"
	clientRM "github.com/brigadecore/brigade/sdk/v2/restmachinery"
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
	}
	var err error
	if config.githubAppID, err =
		os.GetRequiredIntFromEnvVar("GITHUB_APP_ID"); err != nil {
		return config, err
	}
	githubAPIKeyPath := os.GetEnvVar(
		"GITHUB_API_KEY_PATH",
		"/app/github-api-key/github-api-key.pem",
	)
	config.githubAPIKey, err = ioutil.ReadFile(githubAPIKeyPath)
	if err != nil {
		return config, err
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
