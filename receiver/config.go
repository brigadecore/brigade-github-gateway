package main

// nolint: lll
import (
	"io/ioutil"

	"github.com/brigadecore/brigade-foundations/http"
	"github.com/brigadecore/brigade-foundations/os"
	"github.com/brigadecore/brigade-github-gateway/receiver/internal/webhooks"
	"github.com/brigadecore/brigade/sdk/v2/restmachinery"
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
		CheckSuiteAllowedAuthorAssociations: os.GetStringSliceFromEnvVar(
			"CHECK_SUITE_ALLOWED_AUTHOR_ASSOCIATIONS",
			[]string{},
		),
		EmittedEvents: os.GetStringSliceFromEnvVar(
			"EMITTED_EVENTS",
			[]string{"*"},
		),
	}
	var err error
	if config.GithubAppID, err =
		os.GetRequiredIntFromEnvVar("GITHUB_APP_ID"); err != nil {
		return config, err
	}
	githubAPIKeyPath := os.GetEnvVar(
		"GITHUB_API_KEY_PATH",
		"/app/github-api-key/github-api-key.pem",
	)
	if config.GithubAPIKey, err =
		ioutil.ReadFile(githubAPIKeyPath); err != nil {
		return config, err
	}
	if config.CheckSuiteOnPR, err =
		os.GetBoolFromEnvVar("CHECK_SUITE_ON_PR", true); err != nil {
		return config, err
	}
	config.CheckSuiteOnComment, err =
		os.GetBoolFromEnvVar("CHECK_SUITE_ON_COMMENT", true)
	return config, err
}

// webhooksHandlerConfig populates configuration for the (HTTP/S) webhook
// handler from environment variables.
func webhooksHandlerConfig() (webhooks.HandlerConfig, error) {
	config := webhooks.HandlerConfig{}
	var err error
	config.SharedSecret, err = os.GetRequiredEnvVar("GITHUB_APP_SHARED_SECRET")
	return config, err
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
