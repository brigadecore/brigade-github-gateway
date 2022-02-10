package main

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// runHealthcheckLoop checks connectivity between the Monitor and the Brigade
// API server. GitHub is assumed to always be up.
func (m *monitor) runHealthcheckLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.healthcheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := m.systemClient.Ping(ctx, nil); err != nil {
				m.errCh <- errors.Wrap(
					err,
					"error checking Brigade API server connectivity",
				)
			}
		}
	}
}
