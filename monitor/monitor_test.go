package main

import (
	"context"
	"errors"
	"testing"
	"time"

	coreTesting "github.com/brigadecore/brigade/sdk/v2/testing/core"
	systemTesting "github.com/brigadecore/brigade/sdk/v2/testing/system"
	"github.com/stretchr/testify/require"
)

func TestNewMonitor(t *testing.T) {
	m := newMonitor(
		// Totally unusable clients that are enough to fulfill the dependencies for
		// this test...
		&systemTesting.MockAPIClient{},
		&coreTesting.MockEventsClient{
			LogsClient: &coreTesting.MockLogsClient{},
		},
		monitorConfig{},
	)
	require.NotNil(t, m.runHealthcheckLoopFn)
	require.NotNil(t, m.manageEventsFn)
	require.NotNil(t, m.monitorEventFn)
	require.NotNil(t, m.checkRunsClientFactory)
	require.NotNil(t, m.getJobLogsFn)
	require.NotNil(t, m.errFn)
	require.NotNil(t, m.systemClient)
	require.NotNil(t, m.eventsClient)
	require.NotNil(t, m.logsClient)
}

func TestMonitorRun(t *testing.T) {
	testCases := []struct {
		name       string
		setup      func() *monitor
		assertions func(context.Context, error)
	}{
		{
			name: "healthcheck loop produced error",
			setup: func() *monitor {
				errCh := make(chan error)
				return &monitor{
					runHealthcheckLoopFn: func(context.Context) {
						errCh <- errors.New("something went wrong")
					},
					manageEventsFn: func(context.Context) {},
					errCh:          errCh,
				}
			},
			assertions: func(_ context.Context, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
			},
		},
		{
			name: "managing events produced error",
			setup: func() *monitor {
				errCh := make(chan error)
				return &monitor{
					runHealthcheckLoopFn: func(context.Context) {},
					manageEventsFn: func(context.Context) {
						errCh <- errors.New("something went wrong")
					},
					errCh: errCh,
				}
			},
			assertions: func(_ context.Context, err error) {
				require.Error(t, err)
				require.Contains(t, err.Error(), "something went wrong")
			},
		},
		{
			name: "context gets canceled",
			setup: func() *monitor {
				return &monitor{
					runHealthcheckLoopFn: func(context.Context) {},
					manageEventsFn:       func(context.Context) {},
					errCh:                make(chan error),
				}
			},
			assertions: func(ctx context.Context, err error) {
				require.Error(t, err)
				require.Equal(t, ctx.Err(), err)
			},
		},
		{
			name: "timeout during shutdown",
			setup: func() *monitor {
				return &monitor{
					runHealthcheckLoopFn: func(context.Context) {},
					manageEventsFn: func(context.Context) {
						// We'll make this function stubbornly never shut down. Everything
						// should still be ok.
						select {}
					},
					errCh: make(chan error),
				}
			},
			assertions: func(ctx context.Context, err error) {
				require.Error(t, err)
				require.Equal(t, ctx.Err(), err)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := testCase.setup().run(ctx)
			testCase.assertions(ctx, err)
		})
	}
}
