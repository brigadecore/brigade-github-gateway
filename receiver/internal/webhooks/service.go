package webhooks

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	ghlib "github.com/brigadecore/brigade-github-gateway/internal/github"
	"github.com/brigadecore/brigade/sdk/v2/core"
	"github.com/google/go-github/v33/github"
	"github.com/pkg/errors"
)

var (
	branchRefRegex = regexp.MustCompile("refs/heads/(.+)")
	tagRefRegex    = regexp.MustCompile("refs/tags/(.+)")
)

// ServiceConfig encapsulates configuration options for webhook-handling
// service.
type ServiceConfig struct {
	// GithubAppID specifies the ID of a GitHub App.
	GithubAppID int
	// GithubAPIKey is the private API key for the GitHub App specified by the
	// GithubAppID field.
	GithubAPIKey []byte
	// CheckSuiteAllowedAuthorAssociations enumerates the author associations who
	// are allowed to have their PR events and "/brig run" comments trigger the
	// creation of a GitHub CheckSuite. Possible values are: COLLABORATOR,
	// CONTRIBUTOR, OWNER, NONE, MEMBER, FIRST_TIMER, and FIRST_TME_CONTRIBUTOR.
	CheckSuiteAllowedAuthorAssociations []string
	// CheckSuiteOnPR specifies whether eligible PR events (see
	// CheckSuiteAllowedAuthorAssociations) should trigger a corresponding suite
	// of checks. Note that GitHub AUTOMATICALLY triggers such suites in response
	// to push events, but as a security measure, does NOT do so for PR events,
	// given that a PR may have originated from an untrusted user. Setting this
	// field to true, when used in conjunction with the
	// CheckSuiteAllowedAuthorAssociations field allows classes of trusted user
	// (only) to have their PRs trigger check suites automatically.
	CheckSuiteOnPR bool
	// CheckSuiteOnComment specifies whether eligible comments (ones containing
	// the text "/brig run") should trigger a corresponding suite of checks. Note
	// that this privilege is extended only to trusted classes of user specified
	// by the CheckSuiteAllowedAuthorAssociations field.
	CheckSuiteOnComment bool
	// EmittedEvents enumerates specific event types that, when received by the
	// gateway, should be emitted into Brigade's event bus. The value "*" can be
	// used to indicate "all events." ONLY specified events are emitted. i.e. An
	// empty list in this field will result in NO EVENTS being emitted into
	// Brigade's event bus. This field is one of several useful controls for
	// cutting down on the amount of noise that this gateway propagates into
	// Brigade's event bus. (Another would be to configure the Brigade App itself
	// to only send specific events to this gateway.)
	EmittedEvents []string
}

// Service is an interface for components that can handle webhooks (events) from
// GitHub. Implementations of this interface are transport-agnostic.
type Service interface {
	// Handle handles a GitHub webhook (event).
	Handle(
		ctx context.Context,
		eventType string,
		payload []byte,
	) (core.EventList, error)
}

type service struct {
	eventsClient core.EventsClient
	config       ServiceConfig
}

// NewService returns an implementation of the Service interface for handling
// (events) from GitHub.
func NewService(
	eventsClient core.EventsClient,
	config ServiceConfig,
) Service {
	return &service{
		eventsClient: eventsClient,
		config:       config,
	}
}

// nolint: gocyclo
func (s *service) Handle(
	ctx context.Context,
	eventType string,
	payload []byte,
) (core.EventList, error) {
	var events core.EventList

	srcEvent, err := github.ParseWebHook(eventType, payload)
	if err != nil {
		return events, errors.Wrap(err, "error unmarshaling payload")
	}

	brigadeEvent := core.Event{
		Source:  "brigade.sh/github",
		Payload: string(payload),
	}

	// Most of this function is just a giant type switch that extracts relevant
	// details from all the known GitHub webhook (event) types, each of which is
	// represented by its own Go type. For developer convenience, each case links
	// to relevant GitHub API docs.

	switch e := srcEvent.(type) {

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#check_run
	//
	// Check run activity has occurred. The type of activity is specified in the
	// action property of the payload object. For more information, see the "check
	// runs" REST API.
	case *github.CheckRunEvent:
		brigadeEvent.Type = fmt.Sprintf("check_run:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}
		brigadeEvent.Git = &core.GitDetails{
			Commit: e.GetCheckRun().GetCheckSuite().GetHeadSHA(),
			Ref:    e.GetCheckRun().GetCheckSuite().GetHeadBranch(),
		}
		brigadeEvent.SourceState = &core.SourceState{
			State: map[string]string{
				"tracking":       "true",
				"installationID": strconv.FormatInt(e.GetInstallation().GetID(), 10),
				"owner":          e.GetRepo().GetOwner().GetLogin(),
				"repo":           e.GetRepo().GetName(),
				"headSHA":        e.GetCheckRun().GetCheckSuite().GetHeadSHA(),
			},
		}
		if e.GetAction() == "rerequested" {
			if s.shouldEmit(brigadeEvent.Type) {
				if events, err = s.eventsClient.Create(ctx, brigadeEvent); err != nil {
					return events, errors.Wrap(err, "error emitting event(s) into Brigade")
				}
			}
			return events, nil // We're done
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#check_suite
	//
	// Check suite activity has occurred. The type of activity is specified in the
	// action property of the payload object. For more information, see the "check
	// suites" REST API.
	case *github.CheckSuiteEvent:
		brigadeEvent.Type = fmt.Sprintf("check_suite:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}
		brigadeEvent.Git = &core.GitDetails{
			Commit: e.GetCheckSuite().GetHeadSHA(),
			Ref:    e.GetCheckSuite().GetHeadBranch(),
		}
		brigadeEvent.SourceState = &core.SourceState{
			State: map[string]string{
				"tracking":       "true",
				"installationID": strconv.FormatInt(e.GetInstallation().GetID(), 10),
				"owner":          e.GetRepo().GetOwner().GetLogin(),
				"repo":           e.GetRepo().GetName(),
				"headSHA":        e.GetCheckSuite().GetHeadSHA(),
			},
		}
		switch e.GetAction() {
		case "requested", "rerequested":
			if s.shouldEmit(brigadeEvent.Type) {
				if events, err = s.eventsClient.Create(ctx, brigadeEvent); err != nil {
					return events, errors.Wrap(err, "error emitting event(s) into Brigade")
				}
			}
			return events, nil // We're done
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#create
	//
	// A Git branch or tag is created. For more information, see the "Git data"
	// REST API.
	case *github.CreateEvent:
		brigadeEvent.Type = "create"
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}
		brigadeEvent.Git = &core.GitDetails{
			Ref: e.GetRef(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#delete
	//
	// A Git branch or tag is deleted. For more information, see the "Git data"
	// REST API.
	case *github.DeleteEvent:
		brigadeEvent.Type = "delete"
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// // From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#deployment
	// //
	// // A deployment is created. The type of activity is specified in the action
	// // property of the payload object. For more information, see the "deployment"
	// // REST API.
	// case *github.DeploymentEvent:
	// 	// TODO: DeploymentEvent is missing the action property mentioned above.
	// 	// We can add support for this event after opening a PR upstream (or
	// 	// determining that the error is in the documentation, which is a
	// 	// possibility).
	// 	brigadeEvent.Type = fmt.Sprintf("deployment:%s", e.GetAction())
	// 	brigadeEvent.Qualifiers = map[string]string{
	// 		"repo": e.GetRepo().GetFullName(),
	// 	}
	// 	brigadeEvent.Git = &core.GitDetails{
	// 		Commit: e.GetDeployment().GetSHA(),
	// 		Ref:    e.GetDeployment().GetRef(),
	// 	}

	// nolint: lll
	// // From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#deployment_status
	// //
	// // A deployment is created. The type of activity is specified in the action
	// // property of the payload object. For more information, see the "deployment
	// // statuses" REST API.
	// case *github.DeploymentStatusEvent:
	//  // TODO: DeploymentStatusEvent is missing the action property mentioned
	//  // above. We can add support for this event after opening a PR upstream
	//  // (or determining that the error is in the documentation, which is a
	//  // possibility).
	// 	brigadeEvent.Type = fmt.Sprintf("deployment_status:%s", e.GetAction())
	// 	brigadeEvent.Qualifiers = map[string]string{
	// 		"repo": e.GetRepo().GetFullName(),
	// 	}
	// 	brigadeEvent.Git = &core.GitDetails{
	// 		Commit: e.GetDeployment().GetSHA(),
	// 		Ref:    e.GetDeployment().GetRef(),
	// 	}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#fork
	//
	// A user forks a repository. For more information, see the "forks" REST API.
	case *github.ForkEvent:
		brigadeEvent.Type = "fork"
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#github_app_authorization
	//
	// When someone revokes their authorization of a GitHub App, this event
	// occurs. A GitHub App receives this webhook by default and cannot
	// unsubscribe from this event.
	//
	// Anyone can revoke their authorization of a GitHub App from their GitHub
	// account settings page. Revoking the authorization of a GitHub App does not
	// uninstall the GitHub App. You should program your GitHub App so that when
	// it receives this webhook, it stops calling the API on behalf of the person
	// who revoked the token. If your GitHub App continues to use a revoked access
	// token, it will receive the 401 Bad Credentials error. For details about
	// user-to-server requests, which require GitHub App authorization, see
	// "Identifying and authorizing users for GitHub Apps."
	case *github.GitHubAppAuthorizationEvent:
		// We do not want to propagate this event to Brigade, so just bail.
		return events, nil

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#gollum
	//
	// A wiki page is created or updated. For more information, see the "About wikis".
	case *github.GollumEvent:
		brigadeEvent.Type = "gollum"
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#installation
	//
	// Activity related to a GitHub App installation. The type of activity is
	// specified in the action property of the payload object. For more
	// information, see the "GitHub App installation" REST API.
	case *github.InstallationEvent:
		brigadeEvent.Type = fmt.Sprintf("installation:%s", e.GetAction())
		// Special handling for this event type-- an installation can affect
		// multiple repos, so we'll iterate over all affected repos to propagate
		// events for each into Brigade.
		if s.shouldEmit(brigadeEvent.Type) {
			for _, repo := range e.Repositories {
				brigadeEvent.Qualifiers = map[string]string{
					"repo": repo.GetFullName(),
				}
				var tmpEvents core.EventList
				tmpEvents, err = s.eventsClient.Create(ctx, brigadeEvent)
				events.Items = append(events.Items, tmpEvents.Items...)
				if err != nil {
					return events,
						errors.Wrap(err, "error emitting event(s) into Brigade")
				}
			}
		}
		return events, nil // We're done

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#installation_repositories
	//
	// Activity related to repositories being added to a GitHub App installation.
	// The type of activity is specified in the action property of the payload
	// object. For more information, see the "GitHub App installation" REST API.
	case *github.InstallationRepositoriesEvent:
		brigadeEvent.Type =
			fmt.Sprintf("installation_repositories:%s", e.GetAction())
		// Special handling for this event type-- this event can affect multiple
		// repos, so we'll iterate over all affected repos to propagate events for
		// each into Brigade.
		if s.shouldEmit(brigadeEvent.Type) {
			repos := e.RepositoriesAdded
			if e.GetAction() == "removed" {
				repos = e.RepositoriesRemoved
			}
			for _, repo := range repos {
				brigadeEvent.Qualifiers = map[string]string{
					"repo": repo.GetFullName(),
				}
				var tmpEvents core.EventList
				tmpEvents, err = s.eventsClient.Create(ctx, brigadeEvent)
				events.Items = append(events.Items, tmpEvents.Items...)
				if err != nil {
					return events,
						errors.Wrap(err, "error emitting event(s) into Brigade")
				}
			}
		}
		return events, nil // We're done

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#issue_comment
	//
	// Activity related to an issue comment. The type of activity is specified in
	// the action property of the payload object. For more information, see the
	// "issue comments" REST API.
	case *github.IssueCommentEvent:
		brigadeEvent.Type = fmt.Sprintf("issue_comment:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}
		// Under a very specific set of conditions, we will request a check suite to
		// run in response to this comment.
		//
		// 1. The issue in question is a PR
		// 2. The comment contains "/brig check" (case insensitive)
		// 3. Requesting a check suite in response to a comment is enabled
		// 4. The comment's author is allowed to request a check suite
		if e.GetIssue().IsPullRequest() &&
			strings.Contains(
				strings.ToLower(e.GetComment().GetBody()),
				"/brig check",
			) &&
			s.config.CheckSuiteOnComment &&
			s.isAllowedAuthorAssociation(e.GetComment().GetAuthorAssociation()) {
			var pr *github.PullRequest
			if pr, err = s.getPRFromIssueCommentEvent(ctx, *e); err != nil {
				// Log it and continue on. We can (and still should) emit the original
				// event into Brigade.
				log.Println(err)
			} else {
				if err = s.requestCheckSuite(
					ctx,
					e.GetInstallation().GetID(),
					e.GetRepo().GetOwner().GetLogin(),
					e.GetRepo().GetName(),
					pr.GetHead().GetSHA(),
				); err != nil {
					// Log it and continue on. We can (and still should) emit the original
					// event into Brigade.
					log.Println(err)
				}
			}
		}

	// nolint: lll
	// https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#issues
	//
	// Activity related to an issue. The type of activity is specified in the
	// action property of the payload object. For more information, see the
	// "issues" REST API.
	case *github.IssuesEvent:
		brigadeEvent.Type = fmt.Sprintf("issues:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#label
	//
	// Activity related to an issue. The type of activity is specified in the
	// action property of the payload object. For more information, see the
	// "labels" REST API.
	case *github.LabelEvent:
		brigadeEvent.Type = fmt.Sprintf("label:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#member
	//
	// Activity related to repository collaborators. The type of activity is
	// specified in the action property of the payload object. For more
	// information, see the "collaborators" REST API.
	case *github.MemberEvent:
		brigadeEvent.Type = fmt.Sprintf("member:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#milestone
	//
	// Activity related to milestones. The type of activity is specified in the
	// action property of the payload object. For more information, see the
	// "milestones" REST API.
	case *github.MilestoneEvent:
		brigadeEvent.Type = fmt.Sprintf("milestone:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#page_build
	//
	// Represents an attempted build of a GitHub Pages site, whether successful or
	// not. A push to a GitHub Pages enabled branch (gh-pages for project pages,
	// the default branch for user and organization pages) triggers this event.
	case *github.PageBuildEvent:
		brigadeEvent.Type = "page_build"
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#ping
	//
	// When you create a new webhook, we'll send you a simple ping event to let
	// you know you've set up the webhook correctly. This event isn't stored so it
	// isn't retrievable via the Events API endpoint.
	case *github.PingEvent:
		// We do not want to propagate this event to Brigade, so just bail.
		return events, nil

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#project_card
	//
	// Activity related to project cards. The type of activity is specified in the
	// action property of the payload object. For more information, see the
	// "project cards" REST API.
	case *github.ProjectCardEvent:
		brigadeEvent.Type = fmt.Sprintf("project_card:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#project_column
	//
	// Activity related to columns in a project board. The type of activity is
	// specified in the action property of the payload object. For more
	// information, see the "project columns" REST API.
	case *github.ProjectColumnEvent:
		brigadeEvent.Type = fmt.Sprintf("project_column:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#project
	//
	// Activity related to project boards. The type of activity is specified in
	// the action property of the payload object. For more information, see the
	// "projects" REST API.
	case *github.ProjectEvent:
		brigadeEvent.Type = fmt.Sprintf("project:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#public
	//
	// When a private repository is made public. Without a doubt: the best GitHub AE event.
	case *github.PublicEvent:
		brigadeEvent.Type = "public"
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#pull_request
	//
	// Activity related to pull requests. The type of activity is specified in the
	// action property of the payload object. For more information, see the "pull
	// requests" REST API.
	case *github.PullRequestEvent:
		brigadeEvent.Type = fmt.Sprintf("pull_request:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}
		brigadeEvent.ShortTitle, brigadeEvent.LongTitle =
			getTitlesFromPR(e.GetPullRequest())
		brigadeEvent.Git = &core.GitDetails{
			Commit: e.GetPullRequest().GetHead().GetSHA(),
			Ref:    fmt.Sprintf("refs/pull/%d/head", e.GetPullRequest().GetNumber()),
		}
		// Under a very specific set of conditions, we will request a check suite to
		// run in response to this PR.
		//
		// 1. The action is "opened", "synchronize", or "reopened"
		// 2. Requesting a check suite in response to a PR is enabled
		// 3. The PR comes from a fork
		//    a. If the PR does NOT come from a fork, then it comes from a branch
		//       in the same repository to which this PR belongs. If this is the
		//       case, someone necessarily pushed to that branch, and pushes
		//       automatically trigger check suites. Were we to request a check
		//       suite at this juncture, it would be a duplicate.
		// 4. The PR's author is allowed to request a check suite
		switch e.GetAction() {
		case "opened", "synchronize", "reopened":
			if s.config.CheckSuiteOnPR &&
				e.GetPullRequest().GetHead().GetRepo().GetFork() &&
				s.isAllowedAuthorAssociation(e.GetPullRequest().GetAuthorAssociation()) {
				if err = s.requestCheckSuite(
					ctx,
					e.GetInstallation().GetID(),
					e.GetRepo().GetOwner().GetLogin(),
					e.GetRepo().GetName(),
					e.GetPullRequest().GetHead().GetSHA(),
				); err != nil {
					// Log it and continue on. We can (and still should) emit the original
					// event into Brigade.
					log.Println(err)
				}
			}
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#pull_request_review
	//
	// Activity related to pull request reviews. The type of activity is specified
	// in the action property of the payload object. For more information, see the
	// "pull request reviews" REST API.
	case *github.PullRequestReviewEvent:
		brigadeEvent.Type = fmt.Sprintf("pull_request_review:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}
		brigadeEvent.ShortTitle, brigadeEvent.LongTitle =
			getTitlesFromPR(e.GetPullRequest())
		brigadeEvent.Git = &core.GitDetails{
			Commit: e.GetPullRequest().GetHead().GetSHA(),
			Ref:    fmt.Sprintf("refs/pull/%d/head", e.GetPullRequest().GetNumber()),
		}

	// nolint: lll
	// https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#pull_request_review_comment
	//
	// Activity related to pull request review comments in the pull request's
	// unified diff. The type of activity is specified in the action property of
	// the payload object. For more information, see the "pull request review
	// comments" REST API.
	case *github.PullRequestReviewCommentEvent:
		brigadeEvent.Type =
			fmt.Sprintf("pull_request_review_comment:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}
		brigadeEvent.ShortTitle, brigadeEvent.LongTitle =
			getTitlesFromPR(e.GetPullRequest())
		brigadeEvent.Git = &core.GitDetails{
			Commit: e.GetPullRequest().GetHead().GetSHA(),
			Ref:    fmt.Sprintf("refs/pull/%d/head", e.GetPullRequest().GetNumber()),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#push
	//
	// One or more commits are pushed to a repository branch or tag.
	case *github.PushEvent:
		brigadeEvent.Type = "push"
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}
		brigadeEvent.ShortTitle, brigadeEvent.LongTitle = getTitlesFromPushEvent(e)
		brigadeEvent.Git = &core.GitDetails{
			Commit: e.GetHeadCommit().GetID(),
			Ref:    e.GetRef(),
		}
		if e.GetDeleted() {
			// If this is a branch or tag deletion, emit a `push:delete` event
			// instead and blank out the brigadeEvent.Git.
			brigadeEvent.Type = "push:delete"
			brigadeEvent.Git = nil
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#release
	//
	// Activity related to a release. The type of activity is specified in the
	// action property of the payload object. For more information, see the
	// "releases" REST API.
	case *github.ReleaseEvent:
		brigadeEvent.Type = fmt.Sprintf("release:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}
		brigadeEvent.Git = &core.GitDetails{
			Ref: e.GetRelease().GetTagName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#repository
	//
	// Activity related to a repository. The type of activity is specified in the
	// action property of the payload object. For more information, see the
	// "repositories" REST API.
	case *github.RepositoryEvent:
		brigadeEvent.Type = fmt.Sprintf("repository:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// // From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#star
	// //
	// // Activity related to a repository being starred. The type of activity is
	// // specified in the action property of the payload object. For more
	// // information, see the "starring" REST API.
	// case *github.StarEvent:
	// 	brigadeEvent.Type = fmt.Sprintf("star:%s", e.GetAction())
	// 	brigadeEvent.Qualifiers = map[string]string{
	// 		// TODO: StarEvent is missing the repo property. We can add support
	// 		// for this event after opening a PR upstream (or determining that the
	// 		// error is in the documentation, which is a possibility).
	// 		"repo": e.GetRepo().GetFullName(),
	// 	}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#status
	//
	// When the status of a Git commit changes. The type of activity is specified in the action property of the payload object. For more information, see the "statuses" REST API.
	case *github.StatusEvent:
		brigadeEvent.Type = "status"
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}
		brigadeEvent.Git = &core.GitDetails{
			Commit: e.GetCommit().GetSHA(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#team_add
	//
	// When a repository is added to a team.
	case *github.TeamAddEvent:
		brigadeEvent.Type = "team_add"
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	// nolint: lll
	// https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#watch
	//
	// When someone stars a repository. The type of activity is specified in the
	// action property of the payload object. For more information, see the
	// "starring" REST API.
	//
	// The event’s actor is the user who starred a repository, and the event’s
	// repository is the repository that was starred.
	//
	// See https://developer.github.com/changes/2012-09-05-watcher-api/ for
	// more information.
	case *github.WatchEvent:
		brigadeEvent.Type = fmt.Sprintf("watch:%s", e.GetAction())
		brigadeEvent.Qualifiers = map[string]string{
			"repo": e.GetRepo().GetFullName(),
		}

	default:
		return events, nil
	}

	if s.shouldEmit(brigadeEvent.Type) {
		events, err = s.eventsClient.Create(ctx, brigadeEvent)
		if err != nil {
			return events, errors.Wrap(err, "error emitting event(s) into Brigade")
		}
	}

	return events, nil
}

// isAllowedAuthorAssociation makes a determination whether an author having the
// specified relationship to a given repository is permitted to have check
// suites automatically created and executed.
func (s *service) isAllowedAuthorAssociation(authorAssociation string) bool {
	for _, a := range s.config.CheckSuiteAllowedAuthorAssociations {
		if a == authorAssociation {
			return true
		}
	}
	return false
}

// shouldEmit makes a determination whether the specified event type is eligible
// to be emitted into Brigade's event bus.
func (s *service) shouldEmit(eventType string) bool {
	unqualifiedEventType := strings.Split(eventType, ":")[0]
	for _, emitableEvent := range s.config.EmittedEvents {
		if eventType == emitableEvent || unqualifiedEventType == emitableEvent ||
			emitableEvent == "*" {
			return true
		}
	}
	return false
}

// getTitlesFromPushEvent extracts human-readable event titles from a
// github.PushEvent.
func getTitlesFromPushEvent(pe *github.PushEvent) (string, string) {
	var shortTitle, longTitle string
	if pe != nil && pe.Ref != nil {
		if refSubmatches :=
			branchRefRegex.FindStringSubmatch(*pe.Ref); len(refSubmatches) == 2 {
			shortTitle = fmt.Sprintf("branch: %s", refSubmatches[1])
			longTitle = shortTitle
		} else if refSubmatches :=
			tagRefRegex.FindStringSubmatch(*pe.Ref); len(refSubmatches) == 2 {
			shortTitle = fmt.Sprintf("tag: %s", refSubmatches[1])
			longTitle = shortTitle
		}
	}
	return shortTitle, longTitle
}

// getTitlesFromPR extracts human-readable event titles from a
// github.PullRequest.
func getTitlesFromPR(pr *github.PullRequest) (string, string) {
	var shortTitle, longTitle string
	if pr != nil && pr.Number != nil {
		shortTitle = fmt.Sprintf("PR #%d", *pr.Number)
		if pr.Title != nil {
			longTitle = fmt.Sprintf("%s: %s", shortTitle, *pr.Title)
		}
	}
	return shortTitle, longTitle
}

// getPRFromIssueCommentEvent retrieves the github.PullRequest associated with a
// given github.IssueCommentEvent.
func (s *service) getPRFromIssueCommentEvent(
	ctx context.Context,
	ice github.IssueCommentEvent,
) (*github.PullRequest, error) {
	ghClient, err := ghlib.NewClient(
		ctx,
		s.config.GithubAppID,
		ice.GetInstallation().GetID(),
		[]byte(s.config.GithubAPIKey),
	)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"error creating new client for installation %d",
			ice.GetInstallation().GetID(),
		)
	}
	pullRequest, _, err := ghClient.PullRequests.Get(
		ctx,
		ice.GetRepo().GetOwner().GetLogin(),
		ice.GetRepo().GetName(),
		ice.GetIssue().GetNumber(),
	)
	return pullRequest, errors.Wrapf(
		err,
		"error getting pullrequest %d for %s",
		ice.GetIssue().GetNumber(),
		ice.GetRepo().GetFullName(),
	)
}

// requestCheckSuite finds an existing github.CheckSuite for the specified
// commit and requests for it to be re-run. If no such github.CheckSuite exists
// yet, one is created and its initial run is requested.
func (s *service) requestCheckSuite(
	ctx context.Context,
	installationID int64,
	repoOwner string,
	repoName string,
	commit string,
) error {
	ghClient, err := ghlib.NewClient(
		ctx,
		s.config.GithubAppID,
		installationID,
		[]byte(s.config.GithubAPIKey),
	)
	if err != nil {
		return errors.Wrapf(
			err,
			"error creating new client for installation %d",
			installationID,
		)
	}
	// Find existing check suites for this commit
	res, _, err := ghClient.Checks.ListCheckSuitesForRef(
		ctx,
		repoOwner,
		repoName,
		commit,
		&github.ListCheckSuiteOptions{
			// Only list check suites for this appID
			AppID: &s.config.GithubAppID,
		},
	)
	if err != nil {
		return errors.Wrapf(
			err,
			"error listing check suites for commit %s",
			commit,
		)
	}
	var checkSuite *github.CheckSuite
	// We filtered by app ID-- there can only be 0 or 1 results
	if res.GetTotal() > 0 {
		// This is an existing check suite that we can re-run
		checkSuite = res.CheckSuites[0]
	} else {
		// Create a new check suite
		if checkSuite, _, err = ghClient.Checks.CreateCheckSuite(
			ctx,
			repoOwner,
			repoName,
			github.CreateCheckSuiteOptions{
				HeadSHA: commit,
			},
		); err != nil {
			return errors.Wrapf(
				err,
				"error creating check suite for commit %q",
				commit,
			)
		}
	}
	// Run/re-run the check suite to run
	_, err = ghClient.Checks.ReRequestCheckSuite(
		ctx,
		repoOwner,
		repoName,
		checkSuite.GetID(),
	)
	return errors.Wrapf(
		err,
		"error creating check suite for commit %s",
		commit,
	)
}
