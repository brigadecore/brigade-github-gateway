package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/brigadecore/brigade-github-gateway/internal/github"
	"github.com/brigadecore/brigade/sdk/v2/core"
	"github.com/brigadecore/brigade/sdk/v2/system"
)

// monitorConfig encapsulates configuration options for the monitor component.
type monitorConfig struct {
	healthcheckInterval   time.Duration
	listEventsInterval    time.Duration
	eventFollowUpInterval time.Duration
	githubAppID           int
	githubAPIKey          []byte
}

// monitor is a component that continuously monitors certain events that the
// Brigade GitHub Gateway has emitted into Brigade's event bus. As the jobs of
// each event are determined to have been completed, status is reported upstream
// to GitHub.
type monitor struct {
	config monitorConfig
	// All of the monitor's goroutines will send fatal errors here
	errCh chan error
	// All of these internal functions are overridable for testing purposes
	runHealthcheckLoopFn   func(context.Context)
	manageEventsFn         func(context.Context)
	monitorEventFn         func(context.Context, string)
	checkRunsClientFactory github.CheckRunsClientFactory
	getJobLogsFn           func(context.Context, string, core.Job) (string, error)
	errFn                  func(...interface{})
	systemClient           system.APIClient
	eventsClient           core.EventsClient
	logsClient             core.LogsClient
}

// newMonitor initializes and returns a monitor.
func newMonitor(
	systemClient system.APIClient,
	eventsClient core.EventsClient,
	config monitorConfig,
) *monitor {
	m := &monitor{
		config: config,
		errCh:  make(chan error),
	}
	m.runHealthcheckLoopFn = m.runHealthcheckLoop
	m.manageEventsFn = m.manageEvents
	m.monitorEventFn = m.monitorEvent
	m.checkRunsClientFactory = github.NewCheckRunsClientFactory()
	m.getJobLogsFn = m.getJobLogs
	m.errFn = log.Println
	m.systemClient = systemClient
	m.eventsClient = eventsClient
	m.logsClient = eventsClient.Logs()
	return m
}

// run coordinates the many goroutines involved in different aspects of the
// monitor. If any one of these goroutines encounters an unrecoverable error,
// everything shuts down.
func (m *monitor) run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	wg := sync.WaitGroup{}

	// Run healthcheck loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.runHealthcheckLoopFn(ctx)
	}()

	// Check on a regular basis for new events that we should be monitoring
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.manageEventsFn(ctx)
	}()

	// Wait for an error or a completed context
	var err error
	select {
	// If any one loop fails, including the healthcheck, shut everything else
	// down also.
	case err = <-m.errCh:
		cancel() // Shut it all down
	case <-ctx.Done():
		err = ctx.Err()
	}

	// Adapt wg to a channel that can be used in a select
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		wg.Wait()
	}()

	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		// Probably doesn't matter that this is hardcoded. Relatively speaking, 3
		// seconds is a lot of time for things to wrap up.
	}

	return err
}
