package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/brigadecore/brigade/sdk/v2/core"
	coreTesting "github.com/brigadecore/brigade/sdk/v2/testing/core"
	"github.com/google/go-github/v33/github"
	"github.com/stretchr/testify/require"
)

func TestNewService(t *testing.T) {
	s := NewService(
		// Totally unusable client that is enough to fulfill the dependencies for
		// this test...
		&coreTesting.MockEventsClient{
			LogsClient: &coreTesting.MockLogsClient{},
		},
		ServiceConfig{},
	).(*service)
	require.NotNil(t, s.eventsClient)
	require.NotNil(t, s.config)
}

func TestHandle(t *testing.T) {
	testGenericAction := github.String("foo")
	testRepo := &github.Repository{
		FullName: github.String("brigadecore/brigade-github-gateway"),
	}
	const testSHA = "1234567"
	const testBranch = "master"
	testQualifiers := map[string]string{
		"repo": "brigadecore/brigade-github-gateway",
	}

	testCases := []struct {
		name       string
		eventType  string
		eventBytes func() []byte
		service    *service
		assertions func(core.EventList, error)
	}{

		{
			name:      "unknown event type",
			eventType: "bogus",
			eventBytes: func() []byte {
				return []byte("{}")
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(context.Context, core.Event) (core.EventList, error) {
						require.Fail(t, "create event should not have been called")
						return core.EventList{}, nil
					},
				},
			},
			assertions: func(_ core.EventList, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "error unmarshaling payload")
			},
		},

		{
			name:      "bad payload",
			eventType: "check_suite",
			eventBytes: func() []byte {
				return []byte("")
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(context.Context, core.Event) (core.EventList, error) {
						require.Fail(t, "create event should not have been called")
						return core.EventList{}, nil
					},
				},
			},
			assertions: func(_ core.EventList, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "error unmarshaling payload")
			},
		},

		{
			name:      "unsupported event type",
			eventType: "deployment",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(&github.DeploymentEvent{})
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(context.Context, core.Event) (core.EventList, error) {
						require.Fail(t, "create event should not have been called")
						return core.EventList{}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Empty(t, events.Items)
			},
		},

		{
			name:      "check_run event; action is not rerequested",
			eventType: "check_run",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.CheckRunEvent{
						Action: testGenericAction,
						Repo:   testRepo,
						CheckRun: &github.CheckRun{
							CheckSuite: &github.CheckSuite{
								HeadSHA:    github.String(testSHA),
								HeadBranch: github.String(testBranch),
							},
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "check_run:foo", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Commit: testSHA,
						Ref:    testBranch,
					},
					*event.Git,
				)
			},
		},

		{
			name:      "check_run event; action is rerequested",
			eventType: "check_run",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.CheckRunEvent{
						Action: github.String("rerequested"),
						Repo:   testRepo,
						CheckRun: &github.CheckRun{
							CheckSuite: &github.CheckSuite{
								HeadSHA:    github.String(testSHA),
								HeadBranch: github.String(testBranch),
							},
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "check_run:rerequested", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Commit: testSHA,
						Ref:    testBranch,
					},
					*event.Git,
				)
			},
		},

		{
			name:      "check_run event; action is rerequested; create fails",
			eventType: "check_run",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.CheckRunEvent{
						Action: github.String("rerequested"),
						Repo:   testRepo,
						CheckRun: &github.CheckRun{
							CheckSuite: &github.CheckSuite{
								HeadSHA:    github.String(testSHA),
								HeadBranch: github.String(testBranch),
							},
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(context.Context, core.Event) (core.EventList, error) {
						return core.EventList{}, errors.New("something went wrong")
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
				require.Contains(t, err.Error(), "error emitting event(s) into Brigade")
			},
		},

		{
			name:      "check_suite event; action is not requested or rerequested",
			eventType: "check_suite",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.CheckSuiteEvent{
						Action: testGenericAction,
						Repo:   testRepo,
						CheckSuite: &github.CheckSuite{
							HeadSHA:    github.String(testSHA),
							HeadBranch: github.String(testBranch),
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "check_suite:foo", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Commit: testSHA,
						Ref:    testBranch,
					},
					*event.Git,
				)
			},
		},

		{
			name:      "check_suite event; action is requested",
			eventType: "check_suite",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.CheckSuiteEvent{
						Action: github.String("requested"),
						Repo:   testRepo,
						CheckSuite: &github.CheckSuite{
							HeadSHA:    github.String(testSHA),
							HeadBranch: github.String(testBranch),
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "check_suite:requested", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Commit: testSHA,
						Ref:    testBranch,
					},
					*event.Git,
				)
			},
		},

		{
			name:      "check_suite event; action is requested; create fails",
			eventType: "check_suite",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.CheckSuiteEvent{
						Action: github.String("requested"),
						Repo:   testRepo,
						CheckSuite: &github.CheckSuite{
							HeadSHA:    github.String(testSHA),
							HeadBranch: github.String(testBranch),
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(context.Context, core.Event) (core.EventList, error) {
						return core.EventList{}, errors.New("something went wrong")
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
				require.Contains(t, err.Error(), "error emitting event(s) into Brigade")
			},
		},

		{
			name:      "create event",
			eventType: "create",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.CreateEvent{
						Repo: testRepo,
						Ref:  github.String(testBranch),
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "create", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Ref: testBranch,
					},
					*event.Git,
				)
			},
		},

		{
			name:      "delete event",
			eventType: "delete",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.DeleteEvent{
						Repo: testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "delete", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "fork event",
			eventType: "fork",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.ForkEvent{
						Repo: testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "fork", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "github_app_authorization event",
			eventType: "github_app_authorization",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.GitHubAppAuthorizationEvent{},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(context.Context, core.Event) (core.EventList, error) {
						require.Fail(t, "create event should not have been called")
						return core.EventList{}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Empty(t, events.Items)
			},
		},

		{
			name:      "gollum event",
			eventType: "gollum",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.GollumEvent{
						Repo: testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "gollum", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "installation event",
			eventType: "installation",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.InstallationEvent{
						Action: testGenericAction,
						Repositories: []*github.Repository{
							{
								FullName: github.String("brigadecore/brigade-github-gateway"),
							},
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "installation:foo", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "installation event; create fails",
			eventType: "installation",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.InstallationEvent{
						Repositories: []*github.Repository{
							{
								FullName: github.String("brigadecore/brigade-github-gateway"),
							},
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{}, errors.New("something went wrong")
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
				require.Contains(t, err.Error(), "error emitting event(s) into Brigade")
			},
		},

		{
			name:      "installation_repositories event",
			eventType: "installation_repositories",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.InstallationRepositoriesEvent{
						Action: github.String("added"),
						RepositoriesAdded: []*github.Repository{
							{
								FullName: github.String("brigadecore/brigade-github-gateway"),
							},
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "installation_repositories:added", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "installation_repositories event; action is removed",
			eventType: "installation_repositories",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.InstallationRepositoriesEvent{
						Action: github.String("removed"),
						RepositoriesRemoved: []*github.Repository{
							{
								FullName: github.String("brigadecore/brigade-github-gateway"),
							},
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "installation_repositories:removed", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "installation_repositories event; create fails",
			eventType: "installation_repositories",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.InstallationRepositoriesEvent{
						RepositoriesAdded: []*github.Repository{
							{
								FullName: github.String("brigadecore/brigade-github-gateway"),
							},
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(context.Context, core.Event) (core.EventList, error) {
						return core.EventList{}, errors.New("something went wrong")
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
				require.Contains(t, err.Error(), "error emitting event(s) into Brigade")
			},
		},

		{
			name:      "issue_comment event",
			eventType: "issue_comment",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.IssueCommentEvent{
						Action: testGenericAction,
						Repo:   testRepo,
						Issue: &github.Issue{
							PullRequestLinks: nil,
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "issue_comment:foo", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "issues event",
			eventType: "issues",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.IssuesEvent{
						Action: testGenericAction,
						Repo:   testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "issues:foo", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "label event",
			eventType: "label",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.LabelEvent{
						Action: testGenericAction,
						Repo:   testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "label:foo", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "member event",
			eventType: "member",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.MemberEvent{
						Action: testGenericAction,
						Repo:   testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "member:foo", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "milestone event",
			eventType: "milestone",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.MilestoneEvent{
						Action: testGenericAction,
						Repo:   testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "milestone:foo", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "page_build event",
			eventType: "page_build",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.PageBuildEvent{
						Repo: testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "page_build", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "ping event",
			eventType: "ping",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.PingEvent{},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						require.Fail(t, "create event should not have been called")
						return core.EventList{}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Empty(t, events.Items, 0)
			},
		},

		{
			name:      "project_card event",
			eventType: "project_card",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.ProjectCardEvent{
						Action: testGenericAction,
						Repo:   testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "project_card:foo", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "project_column event",
			eventType: "project_column",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.ProjectColumnEvent{
						Action: testGenericAction,
						Repo:   testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "project_column:foo", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "project event",
			eventType: "project",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.ProjectEvent{
						Action: testGenericAction,
						Repo:   testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "project:foo", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "public event",
			eventType: "public",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.PublicEvent{
						Repo: testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "public", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "pull_request event",
			eventType: "pull_request",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.PullRequestEvent{
						Action: testGenericAction,
						Repo:   testRepo,
						PullRequest: &github.PullRequest{
							Number: github.Int(42),
							Head: &github.PullRequestBranch{
								SHA: github.String(testSHA),
							},
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "pull_request:foo", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Commit: testSHA,
						Ref:    "refs/pull/42/head",
					},
					*event.Git,
				)
			},
		},

		{
			name:      "pull_request_review event",
			eventType: "pull_request_review",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.PullRequestReviewEvent{
						Action: testGenericAction,
						Repo:   testRepo,
						PullRequest: &github.PullRequest{
							Number: github.Int(42),
							Head: &github.PullRequestBranch{
								SHA: github.String(testSHA),
							},
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "pull_request_review:foo", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Commit: testSHA,
						Ref:    "refs/pull/42/head",
					},
					*event.Git,
				)
			},
		},

		{
			name:      "pull_request_review_comment event",
			eventType: "pull_request_review_comment",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.PullRequestReviewCommentEvent{
						Action: testGenericAction,
						Repo:   testRepo,
						PullRequest: &github.PullRequest{
							Number: github.Int(42),
							Head: &github.PullRequestBranch{
								SHA: github.String(testSHA),
							},
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "pull_request_review_comment:foo", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Commit: testSHA,
						Ref:    "refs/pull/42/head",
					},
					*event.Git,
				)
			},
		},

		{
			name:      "push event",
			eventType: "push",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.PushEvent{
						Repo: &github.PushEventRepository{
							FullName: github.String("brigadecore/brigade-github-gateway"),
						},
						HeadCommit: &github.HeadCommit{
							ID: github.String(testSHA),
						},
						Ref: github.String(testBranch),
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "push", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Commit: testSHA,
						Ref:    testBranch,
					},
					*event.Git,
				)
			},
		},

		{
			name:      "release event",
			eventType: "release",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.ReleaseEvent{
						Action: testGenericAction,
						Repo:   testRepo,
						Release: &github.RepositoryRelease{
							TagName: github.String("v0.1.0"),
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "release:foo", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Ref: "v0.1.0",
					},
					*event.Git,
				)
			},
		},

		{
			name:      "repository event",
			eventType: "repository",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.RepositoryEvent{
						Action: testGenericAction,
						Repo:   testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "repository:foo", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "status event",
			eventType: "status",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.StatusEvent{
						Repo: testRepo,
						Commit: &github.RepositoryCommit{
							SHA: github.String(testSHA),
						},
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "status", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Commit: testSHA,
					},
					*event.Git,
				)
			},
		},

		{
			name:      "team_add event",
			eventType: "team_add",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.TeamAddEvent{
						Repo: testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "team_add", event.Type)
				require.Nil(t, event.Git)
			},
		},

		{
			name:      "watch event",
			eventType: "watch",
			eventBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.WatchEvent{
						Action: testGenericAction,
						Repo:   testRepo,
					},
				)
				require.NoError(t, err)
				return bytes
			},
			service: &service{
				eventsClient: &coreTesting.MockEventsClient{
					CreateFn: func(
						_ context.Context,
						event core.Event,
					) (core.EventList, error) {
						return core.EventList{
							Items: []core.Event{
								event,
							},
						}, nil
					},
				},
			},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Len(t, events.Items, 1)
				event := events.Items[0]
				require.Equal(t, "watch:foo", event.Type)
				require.Nil(t, event.Git)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.service.config.EmittedEvents = []string{"*"}
			events, err := testCase.service.Handle(
				context.Background(),
				testCase.eventType,
				testCase.eventBytes(),
			)
			for _, event := range events.Items {
				require.Equal(t, "brigade.sh/github", event.Source)
				require.Equal(t, testQualifiers, event.Qualifiers)
			}
			testCase.assertions(events, err)
		})
	}
}

func TestIsAllowedAuthorAssociation(t *testing.T) {
	testCases := []struct {
		name                string
		allowedAssociations []string
		association         string
		expectedResult      bool
	}{
		{
			name:                "not allowed",
			allowedAssociations: []string{"OWNER"},
			association:         "COLLABORATOR",
			expectedResult:      false,
		},
		{
			name:                "allowed",
			allowedAssociations: []string{"OWNER", "COLLABORATOR"},
			association:         "COLLABORATOR",
			expectedResult:      true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			s := &service{
				config: ServiceConfig{
					CheckSuiteAllowedAuthorAssociations: testCase.allowedAssociations,
				},
			}
			require.Equal(
				t,
				testCase.expectedResult,
				s.isAllowedAuthorAssociation(testCase.association),
			)
		})
	}
}

func TestShouldEmit(t *testing.T) {
	testCases := []struct {
		name              string
		allowedEventTypes []string
		eventType         string
		expectedResult    bool
	}{
		{
			name:              "not allowed",
			allowedEventTypes: []string{"pull_request:opened"},
			eventType:         "pull_request:closed",
			expectedResult:    false,
		},
		{
			name:              "exact match",
			allowedEventTypes: []string{"pull_request:opened"},
			eventType:         "pull_request:opened",
			expectedResult:    true,
		},
		{
			name:              "match on unqualified type",
			allowedEventTypes: []string{"pull_request"},
			eventType:         "pull_request:opened",
			expectedResult:    true,
		},
		{
			name:              "wildcard match",
			allowedEventTypes: []string{"*"},
			eventType:         "pull_request:opened",
			expectedResult:    true,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			s := &service{
				config: ServiceConfig{
					EmittedEvents: testCase.allowedEventTypes,
				},
			}
			require.Equal(
				t,
				testCase.expectedResult,
				s.shouldEmit(testCase.eventType),
			)
		})
	}
}

func TestGetTitlesFromPushEvent(t *testing.T) {
	testCases := []struct {
		name               string
		pushEvent          *github.PushEvent
		expectedShortTitle string
		expectedLongTitle  string
	}{
		{
			name:               "nil PushEvent",
			pushEvent:          nil,
			expectedShortTitle: "",
			expectedLongTitle:  "",
		},
		{
			name:               "nil ref",
			pushEvent:          &github.PushEvent{},
			expectedShortTitle: "",
			expectedLongTitle:  "",
		},
		{
			name: "no regex match on ref",
			pushEvent: &github.PushEvent{
				Ref: github.String("foobar"),
			},
			expectedShortTitle: "",
			expectedLongTitle:  "",
		},
		{
			name: "title from branch",
			pushEvent: &github.PushEvent{
				Ref: github.String("refs/heads/foo"),
			},
			expectedShortTitle: "branch: foo",
			expectedLongTitle:  "branch: foo",
		},
		{
			name: "title from tag",
			pushEvent: &github.PushEvent{
				Ref: github.String("refs/tags/foo"),
			},
			expectedShortTitle: "tag: foo",
			expectedLongTitle:  "tag: foo",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			shortTitle, longTitle := getTitlesFromPushEvent(testCase.pushEvent)
			require.Equal(t, testCase.expectedShortTitle, shortTitle)
			require.Equal(t, testCase.expectedLongTitle, longTitle)
		})
	}
}

func TestGetTitlesFromPR(t *testing.T) {
	testCases := []struct {
		name               string
		pullRequest        *github.PullRequest
		expectedShortTitle string
		expectedLongTitle  string
	}{
		{
			name:               "nil PullRequest",
			pullRequest:        nil,
			expectedShortTitle: "",
			expectedLongTitle:  "",
		},
		{
			name:               "nil PR number",
			pullRequest:        &github.PullRequest{},
			expectedShortTitle: "",
			expectedLongTitle:  "",
		},
		{
			name: "non-nil PR number and nil PR title",
			pullRequest: &github.PullRequest{
				Number: github.Int(42),
			},
			expectedShortTitle: "PR #42",
			expectedLongTitle:  "",
		},
		{
			name: "non-nil PR number and non-nil PR title",
			pullRequest: &github.PullRequest{
				Number: github.Int(42),
				Title:  github.String("Life, the universe, and everything"),
			},
			expectedShortTitle: "PR #42",
			expectedLongTitle:  "PR #42: Life, the universe, and everything",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			shortTitle, longTitle := getTitlesFromPR(testCase.pullRequest)
			require.Equal(t, testCase.expectedShortTitle, shortTitle)
			require.Equal(t, testCase.expectedLongTitle, longTitle)
		})
	}
}
