package router

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

func configureTrustedProxies(engine *gin.Engine, proxies []string) error {
	if err := engine.SetTrustedProxies(proxies); err != nil {
		return fmt.Errorf("configure trusted proxies: %w", err)
	}
	return nil
}
