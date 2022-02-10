package main

import (
	"log"

	"github.com/brigadecore/brigade-foundations/signals"
	"github.com/brigadecore/brigade-foundations/version"
	"github.com/brigadecore/brigade/sdk/v3"
)

func main() {
	log.Printf(
		"Starting Brigade GitHub Gateway Monitor -- version %s -- commit %s",
		version.Version(),
		version.Commit(),
	)

	// Brigade System and Events API clients
	var systemClient sdk.SystemClient
	var eventsClient sdk.EventsClient
	{
		address, token, opts, err := apiClientConfig()
		if err != nil {
			log.Fatal(err)
		}
		systemClient = sdk.NewSystemClient(address, token, &opts)
		eventsClient = sdk.NewEventsClient(address, token, &opts)
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
