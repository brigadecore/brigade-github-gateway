package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/armon/circbuf"

	ghlib "github.com/brigadecore/brigade-github-gateway/internal/github"
	"github.com/brigadecore/brigade/sdk/v3"
	"github.com/brigadecore/brigade/sdk/v3/meta"
	"github.com/google/go-github/v33/github"
	"github.com/pkg/errors"
)

const (
	statusCompleted  = "completed"
	statusInProgress = "in_progress"
	statusQueued     = "queued"

	// nolint: misspell
	conclusionCanceled = "cancelled" // This is how GitHub spells it
	conclusionFailure  = "failure"
	conclusionNeutral  = "neutral"
	conclusionSuccess  = "success"
	conclusionTimedOut = "timed_out"
)

func (m *monitor) manageEvents(ctx context.Context) {
	// Maintain a map of functions for canceling the monitoring loop for each of
	// the events we're watching
	loopCancelFns := map[string]func(){}

	ticker := time.NewTicker(m.config.listEventsInterval)
	defer ticker.Stop()

	for {

		// Build a set of current Events. This makes it a little faster and easier
		// to search for Events later in this algorithm.
		currentEvents := map[string]struct{}{}
		listOpts := &meta.ListOptions{Limit: 100}
		for {
			events, err := m.eventsClient.List(
				ctx,
				&sdk.EventsSelector{
					Source: "brigade.sh/github",
					// These are all the phases where something worth reporting might have
					// occurred. Basically, it just excludes pending and canceled phases.
					WorkerPhases: []sdk.WorkerPhase{
						sdk.WorkerPhaseAborted,
						sdk.WorkerPhaseFailed,
						sdk.WorkerPhaseRunning,
						sdk.WorkerPhaseStarting,
						sdk.WorkerPhaseSucceeded,
						sdk.WorkerPhaseTimedOut,
						sdk.WorkerPhaseUnknown,
					},
					SourceState: map[string]string{
						// Only select events that are to be tracked / reported on.
						"tracking": "true",
					},
				},
				listOpts,
			)
			if err != nil {
				select {
				case m.errCh <- errors.Wrap(err, "error listing events"):
				case <-ctx.Done():
				}
				return
			}
			for _, event := range events.Items {
				currentEvents[event.ID] = struct{}{}
			}
			if events.RemainingItemCount > 0 {
				listOpts.Continue = events.Continue
			} else {
				break
			}
		}

		// Reconcile differences between Events we knew about already and the
		// current set of Events...

		// Stop monitoring loops for Events that we don't need to watch anymore.
		// They weren't selected by our query for whatever reason. Maybe they were
		// deleted or aren't tracked anymore because we finished. We really don't
		// care why.
		for eventID, cancelFn := range loopCancelFns {
			if _, stillExists := currentEvents[eventID]; !stillExists {
				cancelFn()
				delete(loopCancelFns, eventID)
			}
		}

		// Start monitoring loops for any new Events that have been discovered
		for eventID := range currentEvents {
			if _, known := loopCancelFns[eventID]; !known {
				loopCtx, loopCtxCancelFn := context.WithCancel(ctx)
				loopCancelFns[eventID] = loopCtxCancelFn
				go m.monitorEventFn(loopCtx, eventID)
			}
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}

}

func (m *monitor) monitorEvent(ctx context.Context, eventID string) {
	log.Printf("monitoring jobs for event %q", eventID)
	defer log.Printf("done monitoring jobs for event %q", eventID)
	if err := m.monitorEventInternal(ctx, eventID); err != nil {
		log.Println(err)
	}
}

func (m *monitor) monitorEventInternal(
	ctx context.Context,
	eventID string,
) error {
	// A map of Job names to GitHub CheckRun IDs
	checkRunIDs := map[string]int64{}
	// The names of all Jobs we have FINISHED reporting on
	reportedComplete := map[string]struct{}{}

	ticker := time.NewTicker(m.config.eventFollowUpInterval)
	defer ticker.Stop()
	for {
		event, err := m.eventsClient.Get(ctx, eventID, nil)
		if err != nil {
			return errors.Wrapf(
				err,
				"error following up on event %q status; giving up",
				eventID,
			)
		}

		appIDStr, ok := event.Labels["appID"]
		if !ok {
			return errors.Errorf(
				"no github app ID found in event %q labels; giving up",
				eventID,
			)
		}
		appID, err := strconv.ParseInt(appIDStr, 10, 64)
		if err != nil {
			return errors.Wrapf(
				err,
				"error parsing github app ID %q from event %q labels; giving up",
				appIDStr,
				eventID,
			)
		}
		app, ok := m.config.gitHubApps[appID]
		if !ok {
			return errors.Errorf(
				"no configuration found for app ID %d from event %q labels; giving up",
				appID,
				eventID,
			)
		}

		// Loop through all of the Event's Jobs and report status for each
		var allJobsCompleted = true
		for _, job := range event.Worker.Jobs {

			// Are we already done reporting on this Job?
			if _, reported := reportedComplete[job.Name]; reported {
				continue // next job
			}

			status, conclusion := m.checkRunStatusAndConclusionFromJobStatus(
				job.Status.Phase,
				job.Spec.Fallible,
			)

			// Note: This will return an empty string if the job isn't in a terminal
			// phase
			jobLogs, err := m.getJobLogsFn(ctx, eventID, job)
			if err != nil {
				return errors.Wrapf(
					err,
					"error getting event %q job %q logs; giving up",
					eventID,
					job.Name,
				)
			}

			installationID, err :=
				strconv.ParseInt(event.SourceState.State["installationID"], 10, 64)
			if err != nil {
				return errors.Wrapf(
					err,
					"error parsing installationID %q from event %q; giving up",
					event.SourceState.State["installationID"],
					eventID,
				)
			}

			// Have we started reporting on this?
			if checkRunID, reported := checkRunIDs[job.Name]; !reported {
				// We HAVEN'T started reporting on this Job, so create a GitHub CheckRun
				if checkRunIDs[job.Name], err = m.createCheckRun(
					ctx,
					app,
					installationID,
					event.SourceState.State["owner"],
					event.SourceState.State["repo"],
					event.SourceState.State["headSHA"],
					event,
					job,
					status,
					conclusion,
					jobLogs,
				); err != nil {
					return errors.Wrap(err, "error creating check run; giving up")
				}
			} else {
				// We HAVE started reporting on this Job, so update the GitHub CheckRun
				if err = m.updateCheckRun(
					ctx,
					app,
					installationID,
					event.SourceState.State["owner"],
					event.SourceState.State["repo"],
					checkRunID,
					event,
					job,
					status,
					conclusion,
					jobLogs,
				); err != nil {
					return errors.Wrap(err, "error updating check run; giving up")
				}
			}
			if job.Status.Phase.IsTerminal() {
				// Record that we're done reporting on this particular Job so that we
				// can skip over it next time we follow up on all of the Event's Jobs
				reportedComplete[job.Name] = struct{}{}
			} else {
				allJobsCompleted = false
			}
		}

		// We are done following up on this Event only after the Event and ALL of
		// its Jobs are in a terminal phase
		if allJobsCompleted && event.Worker.Status.Phase.IsTerminal() {
			// Blank out the Event's source state to reflect that we're done following
			// up on it
			if err := m.eventsClient.UpdateSourceState(
				ctx,
				eventID,
				sdk.SourceState{},
				nil,
			); err != nil {
				return errors.Wrapf(
					err,
					"error clearing source state for event %q; giving up",
					eventID,
				)
			}
			return nil
		}

		// If we didn't return before reaching here, we're going to loop around.
		// Wait first so we're not CONSTANTLY hitting the API server for status
		// updates.
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return nil
		}
	}
}

func (m *monitor) createCheckRun(
	ctx context.Context,
	app ghlib.App,
	installationID int64,
	owner string,
	repo string,
	headSHA string,
	event sdk.Event,
	job sdk.Job,
	status string,
	conclusion string,
	logs string,
) (int64, error) {
	checkRunName := fmt.Sprintf("%s:%s", event.ProjectID, job.Name)
	checkRunOpts := github.CreateCheckRunOptions{
		Name:    checkRunName,
		HeadSHA: headSHA,
		Status:  &status,
	}
	if job.Status.Started != nil {
		checkRunOpts.StartedAt = &github.Timestamp{
			Time: *job.Status.Started,
		}
	}
	if conclusion != "" {
		checkRunOpts.Conclusion = &conclusion
	}
	if job.Status.Ended != nil {
		checkRunOpts.CompletedAt = &github.Timestamp{
			Time: *job.Status.Ended,
		}
	}
	if logs != "" {
		summary := "Job Logs"
		checkRunOpts.Output = &github.CheckRunOutput{
			Title:   &checkRunName,
			Summary: &summary,
			Text:    &logs,
		}
	}
	checkRunsClient, err := m.checkRunsClientFactory.NewCheckRunsClient(
		ctx,
		app.AppID,
		installationID,
		[]byte(app.APIKey),
	)
	if err != nil {
		return 0, err
	}
	checkRun, _, err := checkRunsClient.CreateCheckRun(
		ctx,
		owner,
		repo,
		checkRunOpts,
	)
	return checkRun.GetID(),
		errors.Wrapf(
			err,
			"error creating check run %q for installation %d",
			checkRunOpts.Name,
			installationID,
		)
}

func (m *monitor) updateCheckRun(
	ctx context.Context,
	app ghlib.App,
	installationID int64,
	owner string,
	repo string,
	checkRunID int64,
	event sdk.Event,
	job sdk.Job,
	status string,
	conclusion string,
	logs string,
) error {
	checkRunName := fmt.Sprintf("%s:%s", event.ProjectID, job.Name)
	checkRunOpts := github.UpdateCheckRunOptions{
		Name:   checkRunName,
		Status: &status,
	}
	if conclusion != "" {
		checkRunOpts.Conclusion = &conclusion
	}
	if job.Status.Ended != nil {
		checkRunOpts.CompletedAt = &github.Timestamp{
			Time: *job.Status.Ended,
		}
	}
	if logs != "" {
		summary := "Job Logs"
		checkRunOpts.Output = &github.CheckRunOutput{
			Title:   &checkRunName,
			Summary: &summary,
			Text:    &logs,
		}
	}
	checkRunsClient, err := m.checkRunsClientFactory.NewCheckRunsClient(
		ctx,
		app.AppID,
		installationID,
		[]byte(app.APIKey),
	)
	if err != nil {
		return err
	}
	_, _, err = checkRunsClient.UpdateCheckRun(
		ctx,
		owner,
		repo,
		checkRunID,
		checkRunOpts,
	)
	return errors.Wrapf(
		err,
		"error updating check run %q for installation %d",
		checkRunOpts.Name,
		installationID,
	)
}

func (m *monitor) checkRunStatusAndConclusionFromJobStatus(
	jobPhase sdk.JobPhase,
	fallible bool,
) (string, string) {
	var status string
	var conclusion string
	switch jobPhase {
	case sdk.JobPhaseAborted, sdk.JobPhaseCanceled:
		status = statusCompleted
		conclusion = conclusionCanceled
	case sdk.JobPhaseFailed, sdk.JobPhaseSchedulingFailed, sdk.JobPhaseUnknown: // nolint: lll
		status = statusCompleted
		if fallible && m.config.reportFallibleJobFailuresAsNeutral {
			conclusion = conclusionNeutral
		} else {
			conclusion = conclusionFailure
		}
	case sdk.JobPhasePending, sdk.JobPhaseStarting:
		status = statusQueued
	case sdk.JobPhaseRunning:
		status = statusInProgress
	case sdk.JobPhaseSucceeded:
		status = statusCompleted
		conclusion = conclusionSuccess
	case sdk.JobPhaseTimedOut:
		status = statusCompleted
		conclusion = conclusionTimedOut
	}
	return status, conclusion
}

func (m *monitor) getJobLogs(
	ctx context.Context,
	eventID string,
	job sdk.Job,
) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // Cancel when we return so that we hang up on the log stream!
	if !job.Status.Phase.IsTerminal() {
		return "", nil
	}
	logCh, errCh, err := m.logsClient.Stream(
		ctx,
		eventID,
		&sdk.LogsSelector{
			Job: job.Name,
		},
		nil,
	)
	if err != nil {
		return "", err
	}
	// GitHub places a restriction on the text field for a Check Run at 65535
	// characters, so we use this as a maximum and truncate if needed.
	const maxBytes = 65535
	// This circular buffer can be written to infinitely but maintains a fixed
	// size, preserving the most recent bytes written - this is what we want
	// as we wish to present the last logs written.
	// Ignoring the error as it is only returned if the provided size is <= 0
	jobLogsBuffer, _ := circbuf.NewBuffer(maxBytes)
logLoop:
	for {
		var logEntry sdk.LogEntry
		var ok bool
		select {
		case logEntry, ok = <-logCh:
			if !ok { // The channel was closed. We got everything.
				break logLoop
			}
			if _, err = jobLogsBuffer.Write([]byte(logEntry.Message)); err != nil {
				return "", err
			}
			if _, err = jobLogsBuffer.Write([]byte("\n")); err != nil {
				return "", err
			}
		case err, ok = <-errCh:
			if ok { // Not simply the end of the channel
				return "", err
			}
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if jobLogsBuffer.TotalWritten() > maxBytes {
		logBytes := jobLogsBuffer.Bytes()
		copy(logBytes[0:], "(Previous text omitted)\n")
		return string(logBytes), nil
	}
	return jobLogsBuffer.String(), nil
}
