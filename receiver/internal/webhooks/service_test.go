package webhooks

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/brigadecore/brigade/sdk/v2/core"
	coreTesting "github.com/brigadecore/brigade/sdk/v2/testing/core"
	"github.com/google/go-github/v33/github"
	"github.com/stretchr/testify/require"
)

func TestNewService(t *testing.T) {
	s, ok := NewService(
		// Totally unusable client that is enough to fulfill the dependencies for
		// this test...
		&coreTesting.MockEventsClient{
			LogsClient: &coreTesting.MockLogsClient{},
		},
		ServiceConfig{},
	).(*service)
	require.True(t, ok)
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
		name         string
		webhookType  string
		webhookBytes func() []byte
		service      *service
		assertions   func(core.EventList, error)
	}{

		{
			name:        "unknown webhook type",
			webhookType: "bogus",
			webhookBytes: func() []byte {
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
			name:        "bad payload",
			webhookType: "check_suite",
			webhookBytes: func() []byte {
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
			name:        "unsupported webhook type",
			webhookType: "deployment",
			webhookBytes: func() []byte {
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
			name:        "check_run event with unparsable name",
			webhookType: "check_run",
			webhookBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.CheckRunEvent{
						Action: github.String("rerequested"),
						Repo:   testRepo,
						CheckRun: &github.CheckRun{
							// The check run name below is not of the form <project:job name>
							Name: github.String("foo"),
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
			service: &service{},
			assertions: func(events core.EventList, err error) {
				require.NoError(t, err)
				require.Empty(t, events.Items)
			},
		},

		{
			name:        "check_run webhook",
			webhookType: "check_run",
			webhookBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.CheckRunEvent{
						Action: github.String("rerequested"),
						Repo:   testRepo,
						CheckRun: &github.CheckRun{
							Name: github.String("foo:bar"),
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
				require.Len(t, events.Items, 2)
				event := events.Items[0]
				require.Equal(t, "foo", event.ProjectID)
				require.Equal(t, "check_run:rerequested", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Commit: testSHA,
						Ref:    testBranch,
					},
					*event.Git,
				)
				event = events.Items[1]
				require.Equal(t, "foo", event.ProjectID)
				require.Equal(t, "ci_job:requested", event.Type)
				require.Equal(t, testQualifiers, event.Qualifiers)
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
			name:        "check_suite webhook",
			webhookType: "check_suite",
			webhookBytes: func() []byte {
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
				require.Len(t, events.Items, 2)
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
				event = events.Items[1]
				require.Equal(t, "ci_pipeline:requested", event.Type)
				require.Equal(t, testQualifiers, event.Qualifiers)
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
			name:        "create webhook",
			webhookType: "create",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
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
			name:        "delete webhook",
			webhookType: "delete",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "fork webhook",
			webhookType: "fork",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "github_app_authorization webhook",
			webhookType: "github_app_authorization",
			webhookBytes: func() []byte {
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
			name:        "gollum webhook",
			webhookType: "gollum",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "installation webhook",
			webhookType: "installation",
			webhookBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.InstallationEvent{
						Action: testGenericAction,
						Repositories: []*github.Repository{
							{
								FullName: github.String("brigadecore/brigade"),
							},
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
				require.Len(t, events.Items, 2)
				event := events.Items[0]
				require.Equal(t, "installation:foo", event.Type)
				require.Equal(
					t,
					map[string]string{
						"repo": "brigadecore/brigade",
					},
					event.Qualifiers,
				)
				require.Nil(t, event.Git)
				event = events.Items[1]
				require.Equal(t, "installation:foo", event.Type)
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "installation_repositories webhook",
			webhookType: "installation_repositories",
			webhookBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.InstallationRepositoriesEvent{
						Action: github.String("added"),
						RepositoriesAdded: []*github.Repository{
							{
								FullName: github.String("brigadecore/brigade"),
							},
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
				require.Len(t, events.Items, 2)
				event := events.Items[0]
				require.Equal(t, "installation_repositories:added", event.Type)
				require.Equal(
					t,
					map[string]string{
						"repo": "brigadecore/brigade",
					},
					event.Qualifiers,
				)
				require.Nil(t, event.Git)
				event = events.Items[1]
				require.Equal(t, "installation_repositories:added", event.Type)
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "installation_repositories webhook; action is removed",
			webhookType: "installation_repositories",
			webhookBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.InstallationRepositoriesEvent{
						Action: github.String("removed"),
						RepositoriesRemoved: []*github.Repository{
							{
								FullName: github.String("brigadecore/brigade"),
							},
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
				require.Len(t, events.Items, 2)
				event := events.Items[0]
				require.Equal(t, "installation_repositories:removed", event.Type)
				require.Equal(
					t,
					map[string]string{
						"repo": "brigadecore/brigade",
					},
					event.Qualifiers,
				)
				require.Nil(t, event.Git)
				event = events.Items[1]
				require.Equal(t, "installation_repositories:removed", event.Type)
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "issue_comment webhook",
			webhookType: "issue_comment",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "issues webhook",
			webhookType: "issues",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "label webhook",
			webhookType: "label",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "member webhook",
			webhookType: "member",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "milestone webhook",
			webhookType: "milestone",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "page_build webhook",
			webhookType: "page_build",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "ping webhook",
			webhookType: "ping",
			webhookBytes: func() []byte {
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
			name:        "project_card webhook",
			webhookType: "project_card",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "project_column webhook",
			webhookType: "project_column",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "project webhook",
			webhookType: "project",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "public webhook",
			webhookType: "public",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "pull_request webhook",
			webhookType: "pull_request",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
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
			name:        "pull_request_review webhook",
			webhookType: "pull_request_review",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
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
			name:        "pull_request_review_comment webhook",
			webhookType: "pull_request_review_comment",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
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
			name:        "push webhook",
			webhookType: "push",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
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
			name:        "release webhook",
			webhookType: "release",
			webhookBytes: func() []byte {
				bytes, err := json.Marshal(
					&github.ReleaseEvent{
						Action: github.String("published"),
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
				require.Len(t, events.Items, 2)
				event := events.Items[0]
				require.Equal(t, "release:published", event.Type)
				require.Equal(
					t,
					core.GitDetails{
						Ref: "v0.1.0",
					},
					*event.Git,
				)
				event = events.Items[1]
				require.Equal(t, "cd_pipeline:requested", event.Type)
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
			name:        "repository webhook",
			webhookType: "repository",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "status webhook",
			webhookType: "status",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
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
			name:        "team_add webhook",
			webhookType: "team_add",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},

		{
			name:        "watch webhook",
			webhookType: "watch",
			webhookBytes: func() []byte {
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
				require.Equal(t, testQualifiers, event.Qualifiers)
				require.Nil(t, event.Git)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			events, err := testCase.service.Handle(
				context.Background(),
				42, // Just a fake app ID
				testCase.webhookType,
				testCase.webhookBytes(),
			)
			for _, event := range events.Items {
				require.Equal(t, "brigade.sh/github", event.Source)
			}
			testCase.assertions(events, err)
		})
	}
}

func TestGetTitlesFromPushWebhook(t *testing.T) {
	testCases := []struct {
		name               string
		webhook            *github.PushEvent
		expectedShortTitle string
		expectedLongTitle  string
	}{
		{
			name:               "nil webhook",
			webhook:            nil,
			expectedShortTitle: "",
			expectedLongTitle:  "",
		},
		{
			name:               "nil ref",
			webhook:            &github.PushEvent{},
			expectedShortTitle: "",
			expectedLongTitle:  "",
		},
		{
			name: "no regex match on ref",
			webhook: &github.PushEvent{
				Ref: github.String("foobar"),
			},
			expectedShortTitle: "",
			expectedLongTitle:  "",
		},
		{
			name: "title from branch",
			webhook: &github.PushEvent{
				Ref: github.String("refs/heads/foo"),
			},
			expectedShortTitle: "branch: foo",
			expectedLongTitle:  "branch: foo",
		},
		{
			name: "title from tag",
			webhook: &github.PushEvent{
				Ref: github.String("refs/tags/foo"),
			},
			expectedShortTitle: "tag: foo",
			expectedLongTitle:  "tag: foo",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			shortTitle, longTitle := getTitlesFromPushWebhook(testCase.webhook)
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
