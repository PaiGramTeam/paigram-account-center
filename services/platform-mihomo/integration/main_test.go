//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"platform-mihomo-service/integration/testenv"
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	if _, err := testenv.Bootstrap(ctx); err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "bootstrap integration stack: %v\n", err)
		os.Exit(1)
	}
	cancel()

	code := m.Run()
	teardownCtx, teardownCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	if err := testenv.Teardown(teardownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "teardown integration stack: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	teardownCancel()
	os.Exit(code)
}
