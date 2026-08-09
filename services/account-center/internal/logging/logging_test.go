package logging

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestLogger_ReturnsRealLogger guards against regressing to the silent
// no-op state that zap.L() exhibits in this codebase (zap.ReplaceGlobals
// is never called). If construction via zap.NewProduction fails or the
// accessor falls back to NewNop, .Check at Error level returns nil and
// this test fails — which is exactly what we want to learn about.
func TestLogger_ReturnsRealLogger(t *testing.T) {
	log := Logger()
	require.NotNil(t, log, "Logger() must never return nil")

	// A no-op logger's Check returns nil for every level; a real
	// production logger returns a non-nil CheckedEntry at Error level.
	ce := log.Check(zap.ErrorLevel, "regression-guard probe")
	require.NotNil(t, ce, "Logger() returned a no-op logger; production zap.Logger expected")
}
