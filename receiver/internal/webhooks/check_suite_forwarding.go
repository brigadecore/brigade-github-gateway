package webhooks

import (
	"context"
	"strings"

	ghlib "github.com/brigadecore/brigade-github-gateway/internal/github"
	"github.com/google/go-github/v33/github"
	"github.com/pkg/errors"
)

func (s *service) checkSuiteForwarding(
	ctx context.Context,
	appID int64,
	webhook interface{},
) error {
	switch webhook := webhook.(type) {

	case *github.IssueCommentEvent:
		// Under a very specific set of conditions, we will request a check suite to
		// run in response to this comment.
		//
		// 1. The issue in question is a PR
		// 2. The action is "created"
		// 3. The comment contains "/brig check" or "/brig run" (case insensitive)
		// 4. The comment's author is allowed to request a check suite
		comment := strings.ToLower(webhook.GetComment().GetBody())
		if webhook.GetIssue().IsPullRequest() && webhook.GetAction() == "created" &&
			(strings.Contains(comment, "/brig check") || strings.Contains(comment, "/brig run")) && // nolint: lll
			s.isAllowedAuthorAssociation(webhook.GetComment().GetAuthorAssociation()) { // nolint: lll
			pr, err := s.getPRFromIssueCommentWebhook(
				ctx,
				s.config.GitHubApps[appID],
				*webhook,
			)
			if err != nil {
				return err
			}
			if err = s.requestCheckSuite(
				ctx,
				s.config.GitHubApps[appID],
				webhook.GetInstallation().GetID(),
				webhook.GetRepo().GetOwner().GetLogin(),
				webhook.GetRepo().GetName(),
				pr.GetHead().GetSHA(),
			); err != nil {
				return err
			}
		}

	case *github.PullRequestEvent:
		// Under a very specific set of conditions, we will request a check suite to
		// run in response to this PR.
		//
		// 1. The action is "opened", "synchronize", or "reopened"
		// 2. The PR comes from a fork. (If the PR does NOT come from a fork, then
		//    it comes from a branch in the same repository to which this PR
		//    belongs. If this is the case, someone necessarily pushed to that
		//    branch, and pushes automatically trigger check suites. Were we to
		//    request a check suite at this juncture, it would be a duplicate.)
		// 3. The PR's author is allowed to request a check suite
		switch webhook.GetAction() {
		case "opened", "synchronize", "reopened":
			if webhook.GetPullRequest().GetHead().GetRepo().GetFork() &&
				s.isAllowedAuthorAssociation(webhook.GetPullRequest().GetAuthorAssociation()) { // nolint: lll
				if err := s.requestCheckSuite(
					ctx,
					s.config.GitHubApps[appID],
					webhook.GetInstallation().GetID(),
					webhook.GetRepo().GetOwner().GetLogin(),
					webhook.GetRepo().GetName(),
					webhook.GetPullRequest().GetHead().GetSHA(),
				); err != nil {
					return err
				}
			}
		}

	}

	return nil
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
