package github

import (
	"context"

	"github.com/google/go-github/v33/github"
)

type MockCheckRunsClientFactory struct {
	NewCheckRunsClientFn func(
		ctx context.Context,
		appID int64,
		installationID int64,
		apiKey []byte,
	) (CheckRunsClient, error)
}

func (m *MockCheckRunsClientFactory) NewCheckRunsClient(
	ctx context.Context,
	appID int64,
	installationID int64,
	apiKey []byte,
) (CheckRunsClient, error) {
	return m.NewCheckRunsClientFn(ctx, appID, installationID, apiKey)
}

type MockCheckRunsClient struct {
	CreateCheckRunFn func(
		ctx context.Context,
		owner string,
		repo string,
		opts github.CreateCheckRunOptions,
	) (*github.CheckRun, *github.Response, error)
	UpdateCheckRunFn func(
		ctx context.Context,
		owner string,
		repo string,
		checkRunID int64,
		opts github.UpdateCheckRunOptions,
	) (*github.CheckRun, *github.Response, error)
}

func (m *MockCheckRunsClient) CreateCheckRun(
	ctx context.Context,
	owner string,
	repo string,
	opts github.CreateCheckRunOptions,
) (*github.CheckRun, *github.Response, error) {
	return m.CreateCheckRunFn(ctx, owner, repo, opts)
}

func (m *MockCheckRunsClient) UpdateCheckRun(
	ctx context.Context,
	owner string,
	repo string,
	checkRunID int64,
	opts github.UpdateCheckRunOptions,
) (*github.CheckRun, *github.Response, error) {
	return m.UpdateCheckRunFn(ctx, owner, repo, checkRunID, opts)
}
