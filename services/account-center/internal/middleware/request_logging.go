package middleware

import (
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func RequestLogger(logger *zap.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		fields := correlation.FromContext(c.Request.Context())
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		logFields := []zap.Field{
			zap.String("request_id", fields.RequestID),
			zap.String("trace_id", fields.TraceID),
			zap.String("method", c.Request.Method),
			zap.String("route", route),
			zap.Int("status_code", c.Writer.Status()),
			zap.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		}
		if fields.OperationID != "" {
			logFields = append(logFields, zap.String("operation_id", fields.OperationID))
		}
		switch {
		case c.Writer.Status() >= 500:
			logger.Error("http request completed", logFields...)
		case c.Writer.Status() >= 400:
			logger.Warn("http request completed", logFields...)
		default:
			logger.Info("http request completed", logFields...)
		}
	}
}
