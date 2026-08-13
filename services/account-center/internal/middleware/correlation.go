package middleware

import (
	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/correlation"
	"github.com/gin-gonic/gin"
)

func Correlation() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := correlation.Ensure(c.Request.Context(), correlation.IncomingFields(
			c.Request.Header.Values(correlation.RequestIDHeader),
			c.Request.Header.Values(correlation.TraceParentHeader),
			c.Request.Header.Values(correlation.OperationIDHeader),
		))
		fields := correlation.FromContext(ctx)
		c.Request = c.Request.WithContext(ctx)
		c.Header(correlation.RequestIDHeader, fields.RequestID)
		c.Header(correlation.TraceParentHeader, fields.TraceParent)
		c.Next()
	}
}
