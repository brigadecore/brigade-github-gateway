package main

// nolint: lll
import (
	"encoding/json"
	"io/ioutil"

	"github.com/brigadecore/brigade-foundations/file"
	"github.com/brigadecore/brigade-foundations/http"
	"github.com/brigadecore/brigade-foundations/os"
	"github.com/brigadecore/brigade-github-gateway/internal/github"
	"github.com/brigadecore/brigade-github-gateway/receiver/internal/webhooks"
	"github.com/brigadecore/brigade/sdk/v3/restmachinery"
	"github.com/pkg/errors"
)

// apiClientConfig populates the Brigade SDK's APIClientOptions from
// environment variables.
func apiClientConfig() (string, string, restmachinery.APIClientOptions, error) {
	opts := restmachinery.APIClientOptions{}
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

// webhookServiceConfig populates configuration for the webhook-handling service
// from environment variables.
func webhookServiceConfig() (webhooks.ServiceConfig, error) {
	config := webhooks.ServiceConfig{
		GitHubApps: map[int64]github.App{},
		CheckSuiteAllowedAuthorAssociations: os.GetStringSliceFromEnvVar(
			"CHECK_SUITE_ALLOWED_AUTHOR_ASSOCIATIONS",
			[]string{},
		),
	}
	var err error
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
		config.GitHubApps[githubApp.AppID] = githubApp
	}
	return config, nil
}

// signatureVerificationFilterConfig populates configuration for the signature
// verification filter from environment variables.
func signatureVerificationFilterConfig() (
	webhooks.SignatureVerificationFilterConfig,
	error,
) {
	config := webhooks.SignatureVerificationFilterConfig{
		GitHubApps: map[int64]github.App{},
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
	if err := json.Unmarshal(githubAppsBytes, &githubApps); err != nil {
		return config, err
	}
	for _, githubApp := range githubApps {
		config.GitHubApps[githubApp.AppID] = githubApp
	}
	return config, nil
}

// serverConfig populates configuration for the HTTP/S server from environment
// variables.
func serverConfig() (http.ServerConfig, error) {
	config := http.ServerConfig{}
	var err error
	config.Port, err = os.GetIntFromEnvVar("RECEIVER_PORT", 8080)
	if err != nil {
		return config, err
	}
	config.TLSEnabled, err = os.GetBoolFromEnvVar("TLS_ENABLED", false)
	if err != nil {
		return config, err
	}
	if config.TLSEnabled {
		config.TLSCertPath, err = os.GetRequiredEnvVar("TLS_CERT_PATH")
		if err != nil {
			return config, err
		}
		config.TLSKeyPath, err = os.GetRequiredEnvVar("TLS_KEY_PATH")
		if err != nil {
			return config, err
		}
	}
	return config, nil
}
