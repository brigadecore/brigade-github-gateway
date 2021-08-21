package main

// nolint: lll
import (
	"log"
	"net/http"

	libHTTP "github.com/brigadecore/brigade-foundations/http"
	"github.com/brigadecore/brigade-foundations/signals"
	"github.com/brigadecore/brigade-foundations/version"
	"github.com/brigadecore/brigade-github-gateway/receiver/internal/webhooks"
	"github.com/brigadecore/brigade/sdk/v2/core"
	"github.com/gorilla/mux"
)

func main() {

	log.Printf(
		"Starting Brigade GitHub Gateway Receiver -- version %s -- commit %s",
		version.Version(),
		version.Commit(),
	)

	address, token, opts, err := apiClientConfig()
	if err != nil {
		log.Fatal(err)
	}

	var webhooksService webhooks.Service
	{
		var config webhooks.ServiceConfig
		config, err = webhookServiceConfig()
		if err != nil {
			log.Fatal(err)
		}
		webhooksService = webhooks.NewService(
			core.NewEventsClient(address, token, &opts),
			config,
		)
	}

	var signatureVerificationFilter libHTTP.Filter
	{
		config, err := signatureVerificationFilterConfig()
		if err != nil {
			log.Fatal(err)
		}
		signatureVerificationFilter =
			webhooks.NewSignatureVerificationFilter(config)
	}

	var server libHTTP.Server
	{
		handler := webhooks.NewHandler(webhooksService)
		router := mux.NewRouter()
		router.StrictSlash(true)
		router.Handle(
			"/events",
			http.HandlerFunc( // Make a handler from a function
				signatureVerificationFilter.Decorate(handler.ServeHTTP),
			),
		).Methods(http.MethodPost)
		router.HandleFunc("/healthz", libHTTP.Healthz).Methods(http.MethodGet)
		serverConfig, err := serverConfig()
		if err != nil {
			log.Fatal(err)
		}
		server = libHTTP.NewServer(router, &serverConfig)
	}

	log.Println(
		server.ListenAndServe(signals.Context()),
	)
}
