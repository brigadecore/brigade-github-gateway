package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/brigadecore/brigade/sdk/v3"
	sdkTesting "github.com/brigadecore/brigade/sdk/v3/testing"
	"github.com/stretchr/testify/require"
)

func TestRunHealthcheckLoop(t *testing.T) {
	testCases := []struct {
		name       string
		monitor    *monitor
		assertions func(error)
	}{
		{
			name: "error pinging brigade API server",
			monitor: &monitor{
				systemClient: &sdkTesting.MockSystemClient{
					PingFn: func(
						context.Context,
						*sdk.PingOptions,
					) (sdk.PingResponse, error) {
						return sdk.PingResponse{}, errors.New("something went wrong")
					},
				},
			},
			assertions: func(err error) {
				require.Contains(
					t,
					err.Error(),
					"error checking Brigade API server connectivity",
				)
				require.Contains(t, err.Error(), "something went wrong")
			},
		},
		{
			name: "success",
			monitor: &monitor{
				systemClient: &sdkTesting.MockSystemClient{
					PingFn: func(
						context.Context,
						*sdk.PingOptions,
					) (sdk.PingResponse, error) {
						return sdk.PingResponse{}, nil
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
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			monitor := testCase.monitor
			monitor.config = monitorConfig{healthcheckInterval: time.Second}
			monitor.errCh = make(chan error)
			go monitor.runHealthcheckLoop(ctx)
			// Listen for errors
			select {
			case err := <-monitor.errCh:
				cancel()
				testCase.assertions(err)
			case <-ctx.Done():
			}
			cancel()
		})
	}
}
