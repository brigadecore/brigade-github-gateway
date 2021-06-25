package main

// nolint: lll
import (
	"log"
	"net/http"

	"github.com/brigadecore/brigade-github-gateway/internal/signals"
	"github.com/brigadecore/brigade-github-gateway/internal/version"
	libHTTP "github.com/brigadecore/brigade-github-gateway/receiver/internal/http"
	"github.com/brigadecore/brigade-github-gateway/receiver/internal/system"
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

	var webhooksHandler *webhooks.Handler
	{
		var config webhooks.HandlerConfig
		config, err = webhooksHandlerConfig()
		if err != nil {
			log.Fatal(err)
		}
		webhooksHandler = &webhooks.Handler{
			Service: webhooksService,
			Config:  config,
		}
	}

	var server libHTTP.Server
	{
		router := mux.NewRouter()
		router.StrictSlash(true)
		router.Handle("/events", webhooksHandler).Methods(http.MethodPost)
		router.HandleFunc("/healthz", system.Healthz).Methods(http.MethodGet)
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
