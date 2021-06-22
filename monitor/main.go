package main

import (
	"log"

	"github.com/brigadecore/brigade-github-gateway/internal/signals"
	"github.com/brigadecore/brigade-github-gateway/internal/version"
	"github.com/brigadecore/brigade/sdk/v2/core"
	"github.com/brigadecore/brigade/sdk/v2/system"
)

func main() {
	log.Printf(
		"Starting Brigade GitHub Gateway Monitor -- version %s -- commit %s",
		version.Version(),
		version.Commit(),
	)

	// Brigade System and Events API clients
	var systemClient system.APIClient
	var eventsClient core.EventsClient
	{
		address, token, opts, err := apiClientConfig()
		if err != nil {
			log.Fatal(err)
		}
		systemClient = system.NewAPIClient(address, token, &opts)
		eventsClient = core.NewEventsClient(address, token, &opts)
	}

	var monitor *monitor
	{
		config, err := getMonitorConfig()
		if err != nil {
			log.Fatal(err)
		}
		monitor = newMonitor(systemClient, eventsClient, config)
	}

	// Run it!
	log.Println(monitor.run(signals.Context()))
}
