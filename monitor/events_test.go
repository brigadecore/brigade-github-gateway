package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	ghlib "github.com/brigadecore/brigade-github-gateway/internal/github"
	"github.com/brigadecore/brigade/sdk/v3"
	"github.com/brigadecore/brigade/sdk/v3/meta"
	sdkTesting "github.com/brigadecore/brigade/sdk/v3/testing"
	"github.com/google/go-github/v33/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManageEvents(t *testing.T) {
	testCases := []struct {
		name       string
		monitor    *monitor
		assertions func(error)
	}{
		{
			name: "error listing events",
			monitor: &monitor{
				config: monitorConfig{
					listEventsInterval: time.Second,
				},
				eventsClient: &sdkTesting.MockEventsClient{
					ListFn: func(
						context.Context,
						*sdk.EventsSelector,
						*meta.ListOptions,
					) (sdk.EventList, error) {
						return sdk.EventList{}, errors.New("something went wrong")
					},
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
				require.Contains(t, err.Error(), "error listing events")
			},
		},
		{
			name: "success",
			monitor: &monitor{
				config: monitorConfig{
					listEventsInterval: time.Second,
				},
				eventsClient: &sdkTesting.MockEventsClient{
					ListFn: func(
						context.Context,
						*sdk.EventsSelector,
						*meta.ListOptions,
					) (sdk.EventList, error) {
						return sdk.EventList{
							Items: []sdk.Event{
								{
									ObjectMeta: meta.ObjectMeta{
										ID: "tunguska",
									},
								},
							},
						}, nil
					},
				},
				monitorEventFn: func(context.Context, string) {},
			},
			assertions: func(err error) {
				require.NoError(t, err)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			testCase.monitor.errCh = make(chan error)
			go testCase.monitor.manageEvents(ctx)
			// Listen for errors
			select {
			case err := <-testCase.monitor.errCh:
				testCase.assertions(err)
			case <-ctx.Done():
				testCase.assertions(nil)
			}
			cancel()
		})
	}
}

func TestMonitorEventInternal(t *testing.T) {
	const testEventID = "tunguska"
	var testCheckRunID int64 = 42
	testConfig := monitorConfig{
		eventFollowUpInterval: time.Second,
		gitHubApps: map[int64]ghlib.App{
			86: {
				AppID:  86,
				APIKey: "abcdefg",
			},
		},
	}
	testCases := []struct {
		name       string
		monitor    *monitor
		assertions func(error)
	}{
		{
			name: "error getting event",
			monitor: &monitor{
				config: testConfig,
				eventsClient: &sdkTesting.MockEventsClient{
					GetFn: func(
						context.Context,
						string,
						*sdk.EventGetOptions,
					) (sdk.Event, error) {
						return sdk.Event{}, errors.New("something went wrong")
					},
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
				require.Contains(t, err.Error(), "error following up on event")
			},
		},
		{
			name: "app id missing from event labels",
			monitor: &monitor{
				config: testConfig,
				eventsClient: &sdkTesting.MockEventsClient{
					GetFn: func(
						context.Context,
						string,
						*sdk.EventGetOptions,
					) (sdk.Event, error) {
						return sdk.Event{}, nil
					},
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "no github app ID found in event")
			},
		},
		{
			name: "app id not parseable as int",
			monitor: &monitor{
				config: testConfig,
				eventsClient: &sdkTesting.MockEventsClient{
					GetFn: func(
						context.Context,
						string,
						*sdk.EventGetOptions,
					) (sdk.Event, error) {
						return sdk.Event{
							Labels: map[string]string{
								"appID": "foobar",
							},
						}, nil
					},
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "error parsing github app ID")
			},
		},
		{
			name: "app configuration not found",
			monitor: &monitor{
				config: testConfig,
				eventsClient: &sdkTesting.MockEventsClient{
					GetFn: func(
						context.Context,
						string,
						*sdk.EventGetOptions,
					) (sdk.Event, error) {
						return sdk.Event{
							Labels: map[string]string{
								"appID": "99",
							},
						}, nil
					},
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "no configuration found for app ID")
			},
		},
		{
			name: "error getting job logs",
			monitor: &monitor{
				config: testConfig,
				eventsClient: &sdkTesting.MockEventsClient{
					GetFn: func(
						context.Context,
						string,
						*sdk.EventGetOptions,
					) (sdk.Event, error) {
						return sdk.Event{
							Labels: map[string]string{
								"appID": "86",
							},
							Worker: &sdk.Worker{
								Jobs: []sdk.Job{
									{
										Name: "italian",
										Status: &sdk.JobStatus{
											Phase: sdk.JobPhaseRunning,
										},
									},
								},
							},
						}, nil
					},
				},
				getJobLogsFn: func(context.Context, string, sdk.Job) (string, error) {
					return "", errors.New("something went wrong")
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
				require.Contains(t, err.Error(), "error getting event")
				require.Contains(t, err.Error(), "logs")
			},
		},
		{
			name: "error parsing installation ID",
			monitor: &monitor{
				config: testConfig,
				eventsClient: &sdkTesting.MockEventsClient{
					GetFn: func(
						context.Context,
						string,
						*sdk.EventGetOptions,
					) (sdk.Event, error) {
						return sdk.Event{
							Labels: map[string]string{
								"appID": "86",
							},
							SourceState: &sdk.SourceState{
								State: map[string]string{
									"installationID": "foo", // Cannot be parsed as an int
								},
							},
							Worker: &sdk.Worker{
								Jobs: []sdk.Job{
									{
										Name: "italian",
										Status: &sdk.JobStatus{
											Phase: sdk.JobPhaseRunning,
										},
									},
								},
							},
						}, nil
					},
				},
				getJobLogsFn: func(context.Context, string, sdk.Job) (string, error) {
					return "", nil
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "error parsing installationID")
			},
		},
		{
			name: "error creating check run",
			monitor: &monitor{
				config: testConfig,
				eventsClient: &sdkTesting.MockEventsClient{
					GetFn: func(
						context.Context,
						string, *sdk.EventGetOptions,
					) (sdk.Event, error) {
						return sdk.Event{
							Labels: map[string]string{
								"appID": "86",
							},
							SourceState: &sdk.SourceState{
								State: map[string]string{
									"installationID": strconv.Itoa(int(testCheckRunID)),
								},
							},
							Worker: &sdk.Worker{
								Jobs: []sdk.Job{
									{
										Name: "italian",
										Status: &sdk.JobStatus{
											Phase: sdk.JobPhaseRunning,
										},
									},
								},
							},
						}, nil
					},
				},
				getJobLogsFn: func(context.Context, string, sdk.Job) (string, error) {
					return "", nil
				},
				checkRunsClientFactory: &ghlib.MockCheckRunsClientFactory{
					NewCheckRunsClientFn: func(
						context.Context,
						int64,
						int64,
						[]byte,
					) (ghlib.CheckRunsClient, error) {
						return &ghlib.MockCheckRunsClient{
							CreateCheckRunFn: func(
								context.Context,
								string,
								string,
								github.CreateCheckRunOptions,
							) (*github.CheckRun, *github.Response, error) {
								return nil, nil, errors.New("something went wrong")
							},
						}, nil
					},
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
				require.Contains(t, err.Error(), "error creating check run; giving up")
			},
		},
		{
			name: "error updating check run",
			monitor: &monitor{
				config: testConfig,
				eventsClient: &sdkTesting.MockEventsClient{
					GetFn: func(
						context.Context,
						string,
						*sdk.EventGetOptions,
					) (sdk.Event, error) {
						return sdk.Event{
							Labels: map[string]string{
								"appID": "86",
							},
							SourceState: &sdk.SourceState{
								State: map[string]string{
									"installationID": "42",
								},
							},
							Worker: &sdk.Worker{
								Jobs: []sdk.Job{
									{
										Name: "italian",
										Status: &sdk.JobStatus{
											Phase: sdk.JobPhaseRunning,
										},
									},
								},
							},
						}, nil
					},
				},
				getJobLogsFn: func(context.Context, string, sdk.Job) (string, error) {
					return "", nil
				},
				checkRunsClientFactory: &ghlib.MockCheckRunsClientFactory{
					NewCheckRunsClientFn: func(
						context.Context,
						int64,
						int64,
						[]byte,
					) (ghlib.CheckRunsClient, error) {
						return &ghlib.MockCheckRunsClient{
							CreateCheckRunFn: func(
								context.Context,
								string,
								string,
								github.CreateCheckRunOptions,
							) (*github.CheckRun, *github.Response, error) {
								return &github.CheckRun{
									ID: &testCheckRunID,
								}, nil, nil
							},
							UpdateCheckRunFn: func(
								context.Context,
								string,
								string,
								int64,
								github.UpdateCheckRunOptions,
							) (*github.CheckRun, *github.Response, error) {
								return nil, nil, errors.New("something went wrong")
							},
						}, nil
					},
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
				require.Contains(t, err.Error(), "error updating check run; giving up")
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.monitor.monitorEventInternal(
				context.Background(),
				testEventID,
			)
			testCase.assertions(err)
		})
	}
}

func TestCreateCheckRun(t *testing.T) {
	testApp := ghlib.App{
		AppID:  42,
		APIKey: "foobar",
	}
	const testInstallationID = 99
	const testOwner = "brigadecore"
	const testRepo = "test"
	const testHeadSHA = "123abcd"
	testEvent := sdk.Event{
		ProjectID: "bluebook",
	}
	testStartTime := time.Now()
	testEndTime := testStartTime.Add(time.Minute)
	testJob := sdk.Job{
		Name: "italian",
		Status: &sdk.JobStatus{
			Started: &testStartTime,
			Ended:   &testEndTime,
		},
	}
	const testStatus = statusCompleted
	const testConclusion = conclusionSuccess
	const testLogs = "I am the very model of a modern major-general..."
	var testCheckRunID int64 = 501
	testCases := []struct {
		name       string
		monitor    *monitor
		assertions func(int64, error)
	}{
		{
			name: "error getting check runs client",
			monitor: &monitor{
				checkRunsClientFactory: &ghlib.MockCheckRunsClientFactory{
					NewCheckRunsClientFn: func(
						context.Context,
						int64,
						int64,
						[]byte,
					) (ghlib.CheckRunsClient, error) {
						return nil, errors.New("something went wrong")
					},
				},
			},
			assertions: func(checkRunID int64, err error) {
				require.Equal(t, int64(0), checkRunID)
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
			},
		},
		{
			name: "error creating check run",
			monitor: &monitor{
				checkRunsClientFactory: &ghlib.MockCheckRunsClientFactory{
					NewCheckRunsClientFn: func(
						context.Context,
						int64,
						int64,
						[]byte,
					) (ghlib.CheckRunsClient, error) {
						return &ghlib.MockCheckRunsClient{
							CreateCheckRunFn: func(
								context.Context,
								string,
								string,
								github.CreateCheckRunOptions,
							) (*github.CheckRun, *github.Response, error) {
								return nil, nil, errors.New("something went wrong")
							},
						}, nil
					},
				},
			},
			assertions: func(checkRunID int64, err error) {
				require.Equal(t, int64(0), checkRunID)
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
				require.Contains(t, err.Error(), "error creating check run")
			},
		},
		{
			name: "success",
			monitor: &monitor{
				checkRunsClientFactory: &ghlib.MockCheckRunsClientFactory{
					NewCheckRunsClientFn: func(
						context.Context,
						int64,
						int64,
						[]byte,
					) (ghlib.CheckRunsClient, error) {
						return &ghlib.MockCheckRunsClient{
							CreateCheckRunFn: func(
								_ context.Context,
								owner string,
								repo string,
								opts github.CreateCheckRunOptions,
							) (*github.CheckRun, *github.Response, error) {
								require.Equal(t, testOwner, owner)
								require.Equal(t, testRepo, repo)
								require.Equal(
									t,
									fmt.Sprintf("%s:%s", testEvent.ProjectID, testJob.Name),
									opts.Name,
								)
								require.Equal(t, testHeadSHA, opts.HeadSHA)
								require.Equal(t, testStatus, *opts.Status)
								require.Equal(t, *testJob.Status.Started, opts.StartedAt.Time)
								require.Equal(t, testConclusion, *opts.Conclusion)
								require.Equal(t, *testJob.Status.Ended, opts.CompletedAt.Time)
								require.Equal(
									t,
									fmt.Sprintf("%s:%s", testEvent.ProjectID, testJob.Name),
									*opts.Output.Title,
								)
								require.Equal(t, "Job Logs", *opts.Output.Summary)
								require.Equal(t, testLogs, *opts.Output.Text)
								return &github.CheckRun{ID: &testCheckRunID}, nil, nil
							},
						}, nil
					},
				},
			},
			assertions: func(checkRunID int64, err error) {
				require.Equal(t, testCheckRunID, checkRunID)
				require.NoError(t, err)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			checkRunID, err := testCase.monitor.createCheckRun(
				context.Background(),
				testApp,
				testInstallationID,
				testOwner,
				testRepo,
				testHeadSHA,
				testEvent,
				testJob,
				testStatus,
				testConclusion,
				testLogs,
			)
			testCase.assertions(checkRunID, err)
		})
	}
}

func TestUpdateCheckRun(t *testing.T) {
	testApp := ghlib.App{
		AppID:  42,
		APIKey: "foobar",
	}
	const testInstallationID = 99
	const testOwner = "brigadecore"
	const testRepo = "test"
	testEvent := sdk.Event{
		ProjectID: "bluebook",
	}
	testStartTime := time.Now()
	testEndTime := testStartTime.Add(time.Minute)
	testJob := sdk.Job{
		Name: "italian",
		Status: &sdk.JobStatus{
			Started: &testStartTime,
			Ended:   &testEndTime,
		},
	}
	const testStatus = statusCompleted
	const testConclusion = conclusionSuccess
	const testLogs = "I am the very model of a modern major-general..."
	var testCheckRunID int64 = 501
	testCases := []struct {
		name       string
		monitor    *monitor
		assertions func(error)
	}{
		{
			name: "error getting check runs client",
			monitor: &monitor{
				checkRunsClientFactory: &ghlib.MockCheckRunsClientFactory{
					NewCheckRunsClientFn: func(
						context.Context,
						int64,
						int64,
						[]byte,
					) (ghlib.CheckRunsClient, error) {
						return nil, errors.New("something went wrong")
					},
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
			},
		},
		{
			name: "error updating check run",
			monitor: &monitor{
				checkRunsClientFactory: &ghlib.MockCheckRunsClientFactory{
					NewCheckRunsClientFn: func(
						context.Context,
						int64,
						int64,
						[]byte,
					) (ghlib.CheckRunsClient, error) {
						return &ghlib.MockCheckRunsClient{
							UpdateCheckRunFn: func(
								context.Context,
								string,
								string,
								int64,
								github.UpdateCheckRunOptions,
							) (*github.CheckRun, *github.Response, error) {
								return nil, nil, errors.New("something went wrong")
							},
						}, nil
					},
				},
			},
			assertions: func(err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
				require.Contains(t, err.Error(), "error updating check run")
			},
		},
		{
			name: "success",
			monitor: &monitor{
				checkRunsClientFactory: &ghlib.MockCheckRunsClientFactory{
					NewCheckRunsClientFn: func(
						context.Context,
						int64,
						int64,
						[]byte,
					) (ghlib.CheckRunsClient, error) {
						return &ghlib.MockCheckRunsClient{
							UpdateCheckRunFn: func(
								_ context.Context,
								owner string,
								repo string,
								checkRunID int64,
								opts github.UpdateCheckRunOptions,
							) (*github.CheckRun, *github.Response, error) {
								require.Equal(t, testOwner, owner)
								require.Equal(t, testRepo, repo)
								require.Equal(
									t,
									fmt.Sprintf("%s:%s", testEvent.ProjectID, testJob.Name),
									opts.Name,
								)
								require.Equal(t, testStatus, *opts.Status)
								require.Equal(t, testConclusion, *opts.Conclusion)
								require.Equal(t, *testJob.Status.Ended, opts.CompletedAt.Time)
								require.Equal(
									t,
									fmt.Sprintf("%s:%s", testEvent.ProjectID, testJob.Name),
									*opts.Output.Title,
								)
								require.Equal(t, "Job Logs", *opts.Output.Summary)
								require.Equal(t, testLogs, *opts.Output.Text)
								return &github.CheckRun{ID: &testCheckRunID}, nil, nil
							},
						}, nil
					},
				},
			},
			assertions: func(err error) {
				require.NoError(t, err)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.monitor.updateCheckRun(
				context.Background(),
				testApp,
				testInstallationID,
				testOwner,
				testRepo,
				testCheckRunID,
				testEvent,
				testJob,
				testStatus,
				testConclusion,
				testLogs,
			)
			testCase.assertions(err)
		})
	}
}

func TestCheckRunStatusAndConclusionFromJobStatus(t *testing.T) {
	testCases := []struct {
		jobPhase           sdk.JobPhase
		expectedStatus     string
		expectedConclusion string
	}{
		{
			jobPhase:           sdk.JobPhaseAborted,
			expectedStatus:     statusCompleted,
			expectedConclusion: conclusionCanceled,
		},
		{
			jobPhase:           sdk.JobPhaseCanceled,
			expectedStatus:     statusCompleted,
			expectedConclusion: conclusionCanceled,
		},
		{
			jobPhase:           sdk.JobPhaseFailed,
			expectedStatus:     statusCompleted,
			expectedConclusion: conclusionFailure,
		},
		{
			jobPhase:           sdk.JobPhaseSchedulingFailed,
			expectedStatus:     statusCompleted,
			expectedConclusion: conclusionFailure,
		},
		{
			jobPhase:           sdk.JobPhaseUnknown,
			expectedStatus:     statusCompleted,
			expectedConclusion: conclusionFailure,
		},
		{
			jobPhase:           sdk.JobPhasePending,
			expectedStatus:     statusQueued,
			expectedConclusion: "",
		},
		{
			jobPhase:           sdk.JobPhaseStarting,
			expectedStatus:     statusQueued,
			expectedConclusion: "",
		},
		{
			jobPhase:           sdk.JobPhaseRunning,
			expectedStatus:     statusInProgress,
			expectedConclusion: "",
		},
		{
			jobPhase:           sdk.JobPhaseSucceeded,
			expectedStatus:     statusCompleted,
			expectedConclusion: conclusionSuccess,
		},
		{
			jobPhase:           sdk.JobPhaseTimedOut,
			expectedStatus:     statusCompleted,
			expectedConclusion: conclusionTimedOut,
		},
	}
	for _, testCase := range testCases {
		t.Run(string(testCase.jobPhase), func(t *testing.T) {
			status, conclusion :=
				checkRunStatusAndConclusionFromJobStatus(testCase.jobPhase)
			require.Equal(t, testCase.expectedStatus, status)
			require.Equal(t, testCase.expectedConclusion, conclusion)
		})
	}
}

func TestGetJobLogs(t *testing.T) {
	const testEventID = "123456789"
	testCases := []struct {
		name       string
		monitor    *monitor
		job        sdk.Job
		assertions func(logs string, err error)
	}{
		{
			name:    "job is not terminal",
			monitor: &monitor{},
			job: sdk.Job{
				Status: &sdk.JobStatus{
					Phase: sdk.JobPhaseRunning,
				},
			},
			assertions: func(logs string, err error) {
				require.NoError(t, err)
				require.Empty(t, logs)
			},
		},
		{
			name: "error starting log stream",
			monitor: &monitor{
				logsClient: &sdkTesting.MockLogsClient{
					StreamFn: func(
						context.Context,
						string,
						*sdk.LogsSelector,
						*sdk.LogStreamOptions,
					) (<-chan sdk.LogEntry, <-chan error, error) {
						return nil, nil, errors.New("something went wrong")
					},
				},
			},
			job: sdk.Job{
				Status: &sdk.JobStatus{
					Phase: sdk.JobPhaseSucceeded,
				},
			},
			assertions: func(logs string, err error) {
				require.Error(t, err)
				require.Contains(t, "something went wrong", err.Error())
			},
		},
		{
			name: "error streaming logs",
			monitor: &monitor{
				logsClient: &sdkTesting.MockLogsClient{
					StreamFn: func(
						context.Context,
						string,
						*sdk.LogsSelector,
						*sdk.LogStreamOptions,
					) (<-chan sdk.LogEntry, <-chan error, error) {
						logEntryCh := make(chan sdk.LogEntry)
						errCh := make(chan error)
						go func() {
							errCh <- errors.New("something went wrong")
						}()
						return logEntryCh, errCh, nil
					},
				},
			},
			job: sdk.Job{
				Status: &sdk.JobStatus{
					Phase: sdk.JobPhaseSucceeded,
				},
			},
			assertions: func(logs string, err error) {
				require.Error(t, err)
				require.Contains(t, "something went wrong", err.Error())
			},
		},
		{
			name: "success streaming logs, with truncation",
			monitor: &monitor{
				logsClient: &sdkTesting.MockLogsClient{
					StreamFn: func(
						ctx context.Context,
						_ string,
						_ *sdk.LogsSelector,
						_ *sdk.LogStreamOptions,
					) (<-chan sdk.LogEntry, <-chan error, error) {
						logEntryCh := make(chan sdk.LogEntry)
						errCh := make(chan error)
						go func() {
							// Send 32768 one-char lines for 65536 bytes total
							// (one-char msg + one-char newline)
							for i := 0; i < 32768; i++ {
								select {
								case logEntryCh <- sdk.LogEntry{Message: "l"}:
								case <-ctx.Done():
									return
								}
							}
							close(logEntryCh)
						}()
						return logEntryCh, errCh, nil
					},
				},
			},
			job: sdk.Job{
				Status: &sdk.JobStatus{
					Phase: sdk.JobPhaseSucceeded,
				},
			},
			assertions: func(logs string, err error) {
				require.NoError(t, err)
				assert.Contains(t, logs, "(Previous text omitted)\n")
				assert.Equal(t, len(logs), 65535)
			},
		},
		{
			name: "success streaming logs, no truncation",
			monitor: &monitor{
				logsClient: &sdkTesting.MockLogsClient{
					StreamFn: func(
						ctx context.Context,
						_ string,
						_ *sdk.LogsSelector,
						_ *sdk.LogStreamOptions,
					) (<-chan sdk.LogEntry, <-chan error, error) {
						logEntryCh := make(chan sdk.LogEntry)
						errCh := make(chan error)
						go func() {
							for i := 0; i < 32767; i++ {
								select {
								case logEntryCh <- sdk.LogEntry{Message: "l"}:
								case <-ctx.Done():
									return
								}
							}
							close(logEntryCh)
						}()
						return logEntryCh, errCh, nil
					},
				},
			},
			job: sdk.Job{
				Status: &sdk.JobStatus{
					Phase: sdk.JobPhaseSucceeded,
				},
			},
			assertions: func(logs string, err error) {
				require.NoError(t, err)
				assert.NotContains(t, logs, "(Previous text omitted)\n")
				assert.Equal(t, len(logs), 65534)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			logs, err := testCase.monitor.getJobLogs(
				context.Background(),
				testEventID,
				testCase.job,
			)
			testCase.assertions(logs, err)
		})
	}
}
