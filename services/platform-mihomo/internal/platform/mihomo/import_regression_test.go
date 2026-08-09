package mihomo_test

import (
	"testing"

	platformmihomo "platform-mihomo-service/internal/platform/mihomo"
)

func TestInternalPlatformPackagePathUsesMihomo(t *testing.T) {
	var client platformmihomo.Client = platformmihomo.UnconfiguredClient{}
	if client == nil {
		t.Fatal("expected internal mihomo client package to be available")
	}
}
