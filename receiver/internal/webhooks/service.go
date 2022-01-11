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
	// GitHubApps is a map of GitHub App configurations indexed by App ID.
	GitHubApps map[int64]ghlib.App
	// CheckSuiteAllowedAuthorAssociations enumerates the author associations who
	// are allowed to have their PRs and "/brig check" or "/brig run" comments
	// trigger the creation of a GitHub CheckSuite. Possible values are:
	// COLLABORATOR, CONTRIBUTOR, OWNER, NONE, MEMBER, FIRST_TIMER, and
	// FIRST_TME_CONTRIBUTOR.
	CheckSuiteAllowedAuthorAssociations []string
}

// Service is an interface for components that can handle webhooks from GitHub.
// Implementations of this interface are transport-agnostic.
type Service interface {
	// Handle handles a GitHub webhook.
	Handle(
		ctx context.Context,
		appID int64,
		webhookType string,
		payload []byte,
	) (core.EventList, error)
}

type service struct {
	eventsClient core.EventsClient
	config       ServiceConfig
}

// NewService returns an implementation of the Service interface for handling
// webhooks from GitHub.
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
	appID int64,
	webhookType string,
	payload []byte,
) (core.EventList, error) {
	var events core.EventList

	webhook, err := github.ParseWebHook(webhookType, payload)
	if err != nil {
		return events, errors.Wrap(err, "error unmarshaling payload")
	}

	event := core.Event{
		Source:  "brigade.sh/github",
		Payload: string(payload),
		Labels: map[string]string{
			"appID": strconv.FormatInt(appID, 10),
		},
	}

	// Most of this function is just a giant type switch that extracts relevant
	// details from all the known GitHub webhook types, each of which is
	// represented by its own Go type. For developer convenience, each case links
	// to relevant GitHub API docs.

	switch webhook := webhook.(type) {

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#check_run
	//
	// Check run activity has occurred. The type of activity is specified in the
	// action property of the payload object. For more information, see the "check
	// runs" REST API.
	case *github.CheckRunEvent:
		// A request to re-run a check should be delivered only to the Brigade
		// project that created the corresponding job in the first place, so here we
		// attempt to determine the name of that project.
		jobNameTokens := strings.SplitN(webhook.GetCheckRun().GetName(), ":", 2)
		if len(jobNameTokens) != 2 {
			log.Printf(
				"warning: could not process checkrun:rerequested webhook for job %q",
				webhook.GetCheckRun().GetName(),
			)
			return events, nil
		}
		// NOTE: Targeting a specific project requires Brigade v2.2.0+
		event.ProjectID = jobNameTokens[0]
		event.Type = fmt.Sprintf("check_run:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}
		event.Git = &core.GitDetails{
			Commit: webhook.GetCheckRun().GetCheckSuite().GetHeadSHA(),
			Ref:    webhook.GetCheckRun().GetCheckSuite().GetHeadBranch(),
		}
		if webhook.GetAction() == "rerequested" {
			event.SourceState = &core.SourceState{
				State: map[string]string{
					"tracking": "true",
					"installationID": strconv.FormatInt(
						webhook.GetInstallation().GetID(),
						10,
					),
					"owner":   webhook.GetRepo().GetOwner().GetLogin(),
					"repo":    webhook.GetRepo().GetName(),
					"headSHA": webhook.GetCheckRun().GetCheckSuite().GetHeadSHA(),
				},
			}
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#check_suite
	//
	// Check suite activity has occurred. The type of activity is specified in the
	// action property of the payload object. For more information, see the "check
	// suites" REST API.
	case *github.CheckSuiteEvent:
		event.Type = fmt.Sprintf("check_suite:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}
		event.Git = &core.GitDetails{
			Commit: webhook.GetCheckSuite().GetHeadSHA(),
			Ref:    webhook.GetCheckSuite().GetHeadBranch(),
		}
		if webhook.GetAction() == "requested" ||
			webhook.GetAction() == "rerequested" {
			event.SourceState = &core.SourceState{
				State: map[string]string{
					"tracking": "true",
					"installationID": strconv.FormatInt(
						webhook.GetInstallation().GetID(),
						10,
					),
					"owner":   webhook.GetRepo().GetOwner().GetLogin(),
					"repo":    webhook.GetRepo().GetName(),
					"headSHA": webhook.GetCheckSuite().GetHeadSHA(),
				},
			}
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#create
	//
	// A Git branch or tag is created. For more information, see the "Git data"
	// REST API.
	case *github.CreateEvent:
		event.Type = "create"
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}
		event.Git = &core.GitDetails{
			Ref: webhook.GetRef(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#delete
	//
	// A Git branch or tag is deleted. For more information, see the "Git data"
	// REST API.
	case *github.DeleteEvent:
		event.Type = "delete"
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	// nolint: lll
	// // From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#deployment
	// //
	// // A deployment is created. The type of activity is specified in the action
	// // property of the payload object. For more information, see the "deployment"
	// // REST API.
	// case *github.DeploymentEvent:
	// 	// TODO: DeploymentEvent is missing the action property mentioned above.
	// 	// We can add support for this webhook after opening a PR upstream (or
	// 	// determining that the error is in the documentation, which is a
	// 	// possibility).
	// 	event.Type = fmt.Sprintf("deployment:%s", w.GetAction())
	// 	event.Qualifiers = map[string]string{
	// 		"repo": w.GetRepo().GetFullName(),
	// 	}
	// 	event.Git = &core.GitDetails{
	// 		Commit: w.GetDeployment().GetSHA(),
	// 		Ref:    w.GetDeployment().GetRef(),
	// 	}

	// nolint: lll
	// // From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#deployment_status
	// //
	// // A deployment is created. The type of activity is specified in the action
	// // property of the payload object. For more information, see the "deployment
	// // statuses" REST API.
	// case *github.DeploymentStatusEvent:
	//  // TODO: DeploymentStatusEvent is missing the action property mentioned
	//  // above. We can add support for this webhook after opening a PR upstream
	//  // (or determining that the error is in the documentation, which is a
	//  // possibility).
	// 	event.Type = fmt.Sprintf("deployment_status:%s", w.GetAction())
	// 	event.Qualifiers = map[string]string{
	// 		"repo": w.GetRepo().GetFullName(),
	// 	}
	// 	event.Git = &core.GitDetails{
	// 		Commit: w.GetDeployment().GetSHA(),
	// 		Ref:    w.GetDeployment().GetRef(),
	// 	}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#fork
	//
	// A user forks a repository. For more information, see the "forks" REST API.
	case *github.ForkEvent:
		event.Type = "fork"
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
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
		// We do not want to emit a corresponding event into Brigade's event bus, so
		// just bail.
		return events, nil

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#gollum
	//
	// A wiki page is created or updated. For more information, see the "About
	// wikis".
	case *github.GollumEvent:
		event.Type = "gollum"
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#installation
	//
	// Activity related to a GitHub App installation. The type of activity is
	// specified in the action property of the payload object. For more
	// information, see the "GitHub App installation" REST API.
	case *github.InstallationEvent:
		event.Type = fmt.Sprintf("installation:%s", webhook.GetAction())
		// Special handling for this webhook-- an installation can affect
		// multiple repos, so we'll iterate over all affected repos to emit an event
		// for each into Brigade's event bus.
		for _, repo := range webhook.Repositories {
			event.Qualifiers = map[string]string{
				"repo": repo.GetFullName(),
			}
			var tmpEvents core.EventList
			tmpEvents, err = s.eventsClient.Create(ctx, event)
			events.Items = append(events.Items, tmpEvents.Items...)
			if err != nil {
				return events,
					errors.Wrap(err, "error emitting event(s) into Brigade")
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
		event.Type =
			fmt.Sprintf("installation_repositories:%s", webhook.GetAction())
		// Special handling for this webhook-- installation/uninstallation can
		// affect multiple repos, so we'll iterate over all affected repos to emit
		// an event for each into Brigade's event bus.
		repos := webhook.RepositoriesAdded
		if webhook.GetAction() == "removed" {
			repos = webhook.RepositoriesRemoved
		}
		for _, repo := range repos {
			event.Qualifiers = map[string]string{
				"repo": repo.GetFullName(),
			}
			var tmpEvents core.EventList
			tmpEvents, err = s.eventsClient.Create(ctx, event)
			events.Items = append(events.Items, tmpEvents.Items...)
			if err != nil {
				return events,
					errors.Wrap(err, "error emitting event(s) into Brigade")
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
		event.Type = fmt.Sprintf("issue_comment:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}
		// Under a very specific set of conditions, we will request a check suite to
		// run in response to this comment.
		//
		// 1. The issue in question is a PR
		// 2. The comment contains "/brig check" or "/brig run" (case insensitive)
		// 3. Requesting a check suite in response to a comment is enabled
		// 4. The comment's author is allowed to request a check suite
		comment := strings.ToLower(webhook.GetComment().GetBody())
		if webhook.GetIssue().IsPullRequest() && webhook.GetAction() == "created" &&
			(strings.Contains(comment, "/brig check") || strings.Contains(comment, "/brig run")) && // nolint: lll
			s.isAllowedAuthorAssociation(webhook.GetComment().GetAuthorAssociation()) {
			var pr *github.PullRequest
			if pr, err = s.getPRFromIssueCommentWebhook(
				ctx,
				s.config.GitHubApps[appID],
				*webhook,
			); err != nil {
				// Log it and continue on. We can (and still should) emit the event into
				// Brigade's event bus.
				log.Println(err)
			} else {
				if err = s.requestCheckSuite(
					ctx,
					s.config.GitHubApps[appID],
					webhook.GetInstallation().GetID(),
					webhook.GetRepo().GetOwner().GetLogin(),
					webhook.GetRepo().GetName(),
					pr.GetHead().GetSHA(),
				); err != nil {
					// Log it and continue on. We can (and still should) emit the event
					// into Brigade's event bus.
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
		event.Type = fmt.Sprintf("issues:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#label
	//
	// Activity related to an issue. The type of activity is specified in the
	// action property of the payload object. For more information, see the
	// "labels" REST API.
	case *github.LabelEvent:
		event.Type = fmt.Sprintf("label:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#member
	//
	// Activity related to repository collaborators. The type of activity is
	// specified in the action property of the payload object. For more
	// information, see the "collaborators" REST API.
	case *github.MemberEvent:
		event.Type = fmt.Sprintf("member:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#milestone
	//
	// Activity related to milestones. The type of activity is specified in the
	// action property of the payload object. For more information, see the
	// "milestones" REST API.
	case *github.MilestoneEvent:
		event.Type = fmt.Sprintf("milestone:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#page_build
	//
	// Represents an attempted build of a GitHub Pages site, whether successful or
	// not. A push to a GitHub Pages enabled branch (gh-pages for project pages,
	// the default branch for user and organization pages) triggers this event.
	case *github.PageBuildEvent:
		event.Type = "page_build"
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#ping
	//
	// When you create a new webhook, we'll send you a simple ping event to let
	// you know you've set up the webhook correctly. This event isn't stored so it
	// isn't retrievable via the Events API endpoint.
	case *github.PingEvent:
		// We do not want to emit a corresponding event into Brigade's event bus, so
		// just bail.
		return events, nil

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#project_card
	//
	// Activity related to project cards. The type of activity is specified in the
	// action property of the payload object. For more information, see the
	// "project cards" REST API.
	case *github.ProjectCardEvent:
		event.Type = fmt.Sprintf("project_card:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#project_column
	//
	// Activity related to columns in a project board. The type of activity is
	// specified in the action property of the payload object. For more
	// information, see the "project columns" REST API.
	case *github.ProjectColumnEvent:
		event.Type = fmt.Sprintf("project_column:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#project
	//
	// Activity related to project boards. The type of activity is specified in
	// the action property of the payload object. For more information, see the
	// "projects" REST API.
	case *github.ProjectEvent:
		event.Type = fmt.Sprintf("project:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#public
	//
	// When a private repository is made public. Without a doubt: the best GitHub
	// AE event.
	case *github.PublicEvent:
		event.Type = "public"
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#pull_request
	//
	// Activity related to pull requests. The type of activity is specified in the
	// action property of the payload object. For more information, see the "pull
	// requests" REST API.
	case *github.PullRequestEvent:
		event.Type = fmt.Sprintf("pull_request:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}
		event.ShortTitle, event.LongTitle =
			getTitlesFromPR(webhook.GetPullRequest())
		event.Git = &core.GitDetails{
			Commit: webhook.GetPullRequest().GetHead().GetSHA(),
			Ref:    fmt.Sprintf("refs/pull/%d/head", webhook.GetPullRequest().GetNumber()),
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
		switch webhook.GetAction() {
		case "opened", "synchronize", "reopened":
			if webhook.GetPullRequest().GetHead().GetRepo().GetFork() &&
				s.isAllowedAuthorAssociation(webhook.GetPullRequest().GetAuthorAssociation()) {
				if err = s.requestCheckSuite(
					ctx,
					s.config.GitHubApps[appID],
					webhook.GetInstallation().GetID(),
					webhook.GetRepo().GetOwner().GetLogin(),
					webhook.GetRepo().GetName(),
					webhook.GetPullRequest().GetHead().GetSHA(),
				); err != nil {
					// Log it and continue on. We can (and still should) emit the event
					// into Brigade's event bus.
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
		event.Type = fmt.Sprintf("pull_request_review:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}
		event.ShortTitle, event.LongTitle =
			getTitlesFromPR(webhook.GetPullRequest())
		event.Git = &core.GitDetails{
			Commit: webhook.GetPullRequest().GetHead().GetSHA(),
			Ref:    fmt.Sprintf("refs/pull/%d/head", webhook.GetPullRequest().GetNumber()),
		}

	// nolint: lll
	// https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#pull_request_review_comment
	//
	// Activity related to pull request review comments in the pull request's
	// unified diff. The type of activity is specified in the action property of
	// the payload object. For more information, see the "pull request review
	// comments" REST API.
	case *github.PullRequestReviewCommentEvent:
		event.Type =
			fmt.Sprintf("pull_request_review_comment:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}
		event.ShortTitle, event.LongTitle =
			getTitlesFromPR(webhook.GetPullRequest())
		event.Git = &core.GitDetails{
			Commit: webhook.GetPullRequest().GetHead().GetSHA(),
			Ref:    fmt.Sprintf("refs/pull/%d/head", webhook.GetPullRequest().GetNumber()),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#push
	//
	// One or more commits are pushed to a repository branch or tag.
	case *github.PushEvent:
		event.Type = "push"
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}
		event.ShortTitle, event.LongTitle = getTitlesFromPushWebhook(webhook)
		event.Git = &core.GitDetails{
			Commit: webhook.GetHeadCommit().GetID(),
			Ref:    webhook.GetRef(),
		}
		if webhook.GetDeleted() {
			// If this is a branch or tag deletion, emit a `push:delete` event
			// instead and blank out event.Git.
			event.Type = "push:delete"
			event.Git = nil
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#release
	//
	// Activity related to a release. The type of activity is specified in the
	// action property of the payload object. For more information, see the
	// "releases" REST API.
	case *github.ReleaseEvent:
		event.Type = fmt.Sprintf("release:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}
		event.Git = &core.GitDetails{
			Ref: webhook.GetRelease().GetTagName(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#repository
	//
	// Activity related to a repository. The type of activity is specified in the
	// action property of the payload object. For more information, see the
	// "repositories" REST API.
	case *github.RepositoryEvent:
		event.Type = fmt.Sprintf("repository:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	// nolint: lll
	// // From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#star
	// //
	// // Activity related to a repository being starred. The type of activity is
	// // specified in the action property of the payload object. For more
	// // information, see the "starring" REST API.
	// case *github.StarEvent:
	// 	event.Type = fmt.Sprintf("star:%s", w.GetAction())
	// 	event.Qualifiers = map[string]string{
	// 		// TODO: StarEvent is missing the repo property. We can add support
	// 		// for this webhook after opening a PR upstream (or determining that the
	// 		// error is in the documentation, which is a possibility).
	// 		"repo": w.GetRepo().GetFullName(),
	// 	}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#status
	//
	// When the status of a Git commit changes. The type of activity is specified
	// in the action property of the payload object. For more information, see the
	// "statuses" REST API.
	case *github.StatusEvent:
		event.Type = "status"
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}
		event.Git = &core.GitDetails{
			Commit: webhook.GetCommit().GetSHA(),
		}

	// nolint: lll
	// From https://docs.github.com/en/github-ae@latest/developers/webhooks-and-events/webhook-events-and-payloads#team_add
	//
	// When a repository is added to a team.
	case *github.TeamAddEvent:
		event.Type = "team_add"
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
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
		event.Type = fmt.Sprintf("watch:%s", webhook.GetAction())
		event.Qualifiers = map[string]string{
			"repo": webhook.GetRepo().GetFullName(),
		}

	default:
		return events, nil
	}

	events, err = s.eventsClient.Create(ctx, event)
	if err != nil {
		return events, errors.Wrap(err, "error emitting event(s) into Brigade")
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

// getTitlesFromPushWebhook extracts human-readable titles from a
// github.PushEvent.
func getTitlesFromPushWebhook(pe *github.PushEvent) (string, string) {
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

// getTitlesFromPR extracts human-readable titles from a
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

// getPRFromIssueCommentWebhook retrieves the github.PullRequest associated with
// a given github.IssueCommentEvent.
func (s *service) getPRFromIssueCommentWebhook(
	ctx context.Context,
	app ghlib.App,
	ice github.IssueCommentEvent,
) (*github.PullRequest, error) {
	ghClient, err := ghlib.NewClient(
		ctx,
		app.AppID,
		ice.GetInstallation().GetID(),
		[]byte(app.APIKey),
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
	app ghlib.App,
	installationID int64,
	repoOwner string,
	repoName string,
	commit string,
) error {
	ghClient, err := ghlib.NewClient(
		ctx,
		app.AppID,
		installationID,
		[]byte(app.APIKey),
	)
	if err != nil {
		return errors.Wrapf(
			err,
			"error creating new client for installation %d",
			installationID,
		)
	}
	// Find existing check suites for this commit
	appID := int(app.AppID)
	res, _, err := ghClient.Checks.ListCheckSuitesForRef(
		ctx,
		repoOwner,
		repoName,
		commit,
		&github.ListCheckSuiteOptions{
			// Only list check suites for this appID
			AppID: &appID,
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
