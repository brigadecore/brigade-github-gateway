package github

import (
	"context"

	"github.com/google/go-github/v33/github"
	"github.com/pkg/errors"
)

type CheckRunsClientFactory interface {
	NewCheckRunsClient(
		ctx context.Context,
		appID int64,
		installationID int64,
		apiKey []byte,
	) (CheckRunsClient, error)
}

type checkRunsClientFactory struct{}

func NewCheckRunsClientFactory() CheckRunsClientFactory {
	return &checkRunsClientFactory{}
}

func (c *checkRunsClientFactory) NewCheckRunsClient(
	ctx context.Context,
	appID int64,
	installationID int64,
	apiKey []byte,
) (CheckRunsClient, error) {
	ghClient, err := NewClient(ctx, appID, installationID, apiKey)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"error creating new client for installation %d",
			installationID,
		)
	}
	return ghClient.Checks, nil
}

type CheckRunsClient interface {
	CreateCheckRun(
		ctx context.Context,
		owner string,
		repo string,
		opts github.CreateCheckRunOptions,
	) (*github.CheckRun, *github.Response, error)
	UpdateCheckRun(
		ctx context.Context,
		owner string,
		repo string,
		checkRunID int64,
		opts github.UpdateCheckRunOptions,
	) (*github.CheckRun, *github.Response, error)
}
