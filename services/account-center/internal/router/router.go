package router

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/servicehealth"
	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	"gorm.io/gorm"

	"paigram/internal/config"
	"paigram/internal/email"
	"paigram/internal/handler"
	authhandler "paigram/internal/handler/auth"
	"paigram/internal/healthcheck"
	"paigram/internal/httpserver"
	"paigram/internal/logging"
	"paigram/internal/middleware"
	"paigram/internal/observability"
	"paigram/internal/platformtransport"
	"paigram/internal/response"
	"paigram/internal/service"
	"paigram/internal/serviceticket"
	"paigram/internal/sessioncache"
)

// New initialises the Gin router with application routes.
func New(cfg *config.Config, cache sessioncache.Store, db *gorm.DB, rateLimitStore limiter.Store, emailService *email.Service) (*gin.Engine, error) {
	ticketSigner, err := serviceticket.NewFileSigner(
		cfg.Auth.ServiceTicketIssuer,
		time.Duration(cfg.Auth.ServiceTicketTTLSeconds)*time.Second,
		cfg.Auth.ServiceTicketSigningKeyFile,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize service ticket signer: %w", err)
	}
	return NewWithTicketSigner(cfg, cache, db, rateLimitStore, emailService, ticketSigner)
}

func NewWithTicketSigner(cfg *config.Config, cache sessioncache.Store, db *gorm.DB, rateLimitStore limiter.Store, emailService *email.Service, ticketSigner serviceticket.Signer) (*gin.Engine, error) {
	return NewWithTicketSignerAndReadiness(cfg, cache, db, rateLimitStore, emailService, ticketSigner, healthcheck.NewReadiness(db, nil, false))
}

func NewWithTicketSignerAndReadiness(cfg *config.Config, cache sessioncache.Store, db *gorm.DB, rateLimitStore limiter.Store, emailService *email.Service, ticketSigner serviceticket.Signer, readiness servicehealth.Checker) (*gin.Engine, error) {
	controlDialer, err := platformtransport.NewControlDialer(platformtransport.ControlConfig{
		RootCAFile: cfg.PlatformControl.RootCAFile, CertificateFile: cfg.PlatformControl.CertificateFile,
		PrivateKeyFile: cfg.PlatformControl.PrivateKeyFile, ServerName: cfg.PlatformControl.ServerName,
		Timeout: cfg.PlatformControl.DialTimeout,
	})
	if err != nil {
		return nil, err
	}
	return NewWithRuntimeDependenciesAndReadiness(cfg, cache, db, rateLimitStore, emailService, ticketSigner, controlDialer, readiness)
}

func NewWithRuntimeDependencies(cfg *config.Config, cache sessioncache.Store, db *gorm.DB, rateLimitStore limiter.Store, emailService *email.Service, ticketSigner serviceticket.Signer, controlDialer platformtransport.DialFunc) (*gin.Engine, error) {
	return NewWithRuntimeDependenciesAndReadiness(cfg, cache, db, rateLimitStore, emailService, ticketSigner, controlDialer, healthcheck.NewReadiness(db, nil, false))
}

func NewWithRuntimeDependenciesAndReadiness(cfg *config.Config, cache sessioncache.Store, db *gorm.DB, rateLimitStore limiter.Store, emailService *email.Service, ticketSigner serviceticket.Signer, controlDialer platformtransport.DialFunc, readiness servicehealth.Checker) (*gin.Engine, error) {
	appCfg := cfg.App
	authCfg := cfg.Auth
	rateLimitCfg := cfg.RateLimit

	if appCfg.Mode == "" {
		appCfg.Mode = gin.ReleaseMode
	}
	gin.SetMode(appCfg.Mode)

	engine := gin.New()
	engine.Use(middleware.Correlation())
	if sentryMiddleware := observability.GinMiddleware(cfg.Sentry); sentryMiddleware != nil {
		engine.Use(sentryMiddleware)
	}
	if scopeMiddleware := observability.GinScopeMiddleware(); scopeMiddleware != nil {
		engine.Use(scopeMiddleware)
	}
	engine.Use(middleware.RequestLogger(logging.Logger()), gin.Recovery())
	engine.GET("/metrics", gin.WrapH(observability.NewMetricsHandler(db, []observability.CertificateTarget{
		{Identity: "grpc-server", CertificateFile: cfg.GRPC.CertificateFile},
		{Identity: "platform-control-client", CertificateFile: cfg.PlatformControl.CertificateFile},
		{Identity: "platform-control-trust", CertificateFile: cfg.PlatformControl.RootCAFile},
	})))

	// V10: emit baseline security response headers BEFORE CORS so they
	// are present even on CORS-rejected and error responses.
	engine.Use(middleware.SecurityHeaders(middleware.SecurityHeadersConfig{
		HSTSMaxAgeSeconds: cfg.Security.SecurityHeaders.HSTSMaxAgeSeconds,
		HSTSIncludeSub:    cfg.Security.SecurityHeaders.HSTSIncludeSub,
		CSP:               cfg.Security.SecurityHeaders.CSP,
		AssumeHTTPS:       cfg.Security.SecurityHeaders.AssumeHTTPS,
	}))

	corsMiddleware, err := newCORSMiddleware(appCfg.CORS)
	if err != nil {
		log.Printf("[SECURITY WARNING] Failed to configure CORS middleware: %v", err)
	} else if corsMiddleware != nil {
		engine.Use(corsMiddleware)
		log.Printf("[SECURITY] CORS enabled for %v", appCfg.CORS.AllowOrigins)
	}

	if err := configureTrustedProxies(engine, appCfg.TrustedProxies); err != nil {
		return nil, err
	}
	if len(appCfg.TrustedProxies) > 0 {
		log.Printf("[security] app.trusted_proxies = %v", appCfg.TrustedProxies)
	} else {
		log.Printf("[security] app.trusted_proxies is empty; forwarded client addresses are ignored")
	}

	runtime, err := httpserver.Attach(engine, httpserver.Options{
		Title:   appCfg.Name + " API",
		Version: "1.0.0",
		Body: httpserver.BodyOptions{
			MaxBytes:    appCfg.MaxBodyBytes,
			ReadTimeout: appCfg.BodyReadTimeout,
		},
		OpenAPI: httpserver.OpenAPIOptions{
			Enabled: cfg.OpenAPI.Enabled,
			Path:    cfg.OpenAPI.Path,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("attach Huma runtime: %w", err)
	}

	registerHealthRoutes(engine, readiness)

	v1 := runtime.V1

	// Initialize handler groups with dependencies (also seeds loginrisk + geolocation subgroups).
	if err := handler.InitializeApiGroupsWithTransport(db, cache, authCfg, cfg.Frontend, cfg.Security, cfg.TelegramOIDC, ticketSigner, controlDialer); err != nil {
		return nil, fmt.Errorf("initialize api groups: %w", err)
	}
	handler.ApiGroupApp.AuthApiGroup = *authhandler.NewApiGroup(
		db, authCfg, cfg.Frontend, emailService, cfg.Security, cache,
		&service.ServiceGroupApp.GeolocationServiceGroup,
		&service.ServiceGroupApp.LoginRiskServiceGroup,
	)
	RouterGroupApp.OAuthRouterGroup.RegisterPublic(v1)
	// Phase 5 Sub-project 1: mount /auth/telegram/start + /auth/telegram/callback
	// on the unauthenticated v1 group. Both handlers ARE session establishment
	// endpoints; wrapping them in AuthMiddleware would create a chicken-and-egg
	// deadlock. When telegram_oidc credentials are unset at boot, the underlying
	// TelegramOIDCApiGroup.OIDC pointer is nil and these routes are NOT mounted.
	// Spec: docs/superpowers/specs/2026-06-06-phase5-sub1-telegram-oidc-bot-link.md §5.5
	if handler.ApiGroupApp.TelegramOIDCApiGroup.OIDC != nil {
		RouterGroupApp.TelegramOIDCRouterGroup.RegisterPublic(v1)
	}

	// Public routes - no authentication required
	authHandler := &handler.ApiGroupApp.AuthApiGroup.Handler
	authGroup := v1.Group("/auth")
	rateLimitingEnabled := rateLimitCfg.Enabled && rateLimitStore != nil
	registerAuthContracts(authGroup, rateLimitingEnabled)
	{
		// Apply rate limiting to auth endpoints if enabled
		if rateLimitCfg.Enabled && rateLimitStore != nil {
			// Register endpoint with rate limiting
			authGroup.POST("/register",
				middleware.RateLimit(middleware.RateLimitConfig{
					Rate:    rateLimitCfg.Auth.Register,
					KeyFunc: middleware.IPKeyFunc,
					Store:   rateLimitStore,
				}),
				authHandler.RegisterEmail,
			)

			// Login endpoint with rate limiting
			authGroup.POST("/login",
				middleware.RateLimit(middleware.RateLimitConfig{
					Rate:    rateLimitCfg.Auth.Login,
					KeyFunc: middleware.IPKeyFunc,
					Store:   rateLimitStore,
				}),
				authHandler.LoginWithEmail,
			)

			// Refresh token endpoint with rate limiting
			authGroup.POST("/refresh",
				middleware.RateLimit(middleware.RateLimitConfig{
					Rate:    rateLimitCfg.Auth.RefreshToken,
					KeyFunc: middleware.IPKeyFunc,
					Store:   rateLimitStore,
				}),
				authHandler.RefreshToken,
			)

			// Verify email endpoint with rate limiting (by email)
			authGroup.POST("/verify-email",
				middleware.RateLimit(middleware.RateLimitConfig{
					Rate:    rateLimitCfg.Auth.VerifyEmail,
					KeyFunc: middleware.EmailKeyFunc("email"),
					Store:   rateLimitStore,
				}),
				authHandler.VerifyEmail,
			)

			// Password reset request endpoint with rate limiting (by email)
			authGroup.POST("/forgot-password",
				middleware.RateLimit(middleware.RateLimitConfig{
					Rate:    rateLimitCfg.Auth.VerifyEmail,
					KeyFunc: middleware.EmailKeyFunc("email"),
					Store:   rateLimitStore,
				}),
				authHandler.ForgotPassword,
			)

			// Password reset completion endpoint with rate limiting (by IP)
			authGroup.POST("/reset-password",
				middleware.RateLimit(middleware.RateLimitConfig{
					Rate:    rateLimitCfg.API.Unauthenticated,
					KeyFunc: middleware.IPKeyFunc,
					Store:   rateLimitStore,
				}),
				authHandler.ResetPassword,
			)

			// Logout doesn't need strict rate limiting
			authGroup.POST("/logout", authHandler.Logout)

			// OAuth routes with rate limiting
			oauth := authGroup.Group("/oauth")
			oauth.Use(middleware.RateLimit(middleware.RateLimitConfig{
				Rate:    rateLimitCfg.Auth.OAuth,
				KeyFunc: middleware.IPKeyFunc,
				Store:   rateLimitStore,
			}))
			{
				authHandler.RegisterOAuth(oauth)
			}
		} else {
			// No rate limiting - register routes normally
			authHandler.Register(authGroup)
		}
	}

	// Protected routes - require authentication
	protected := v1.Group("").WithAccess(httpserver.Access{Authenticated: true})
	protected.Use(middleware.AuthMiddleware(cache, authCfg))

	// Apply rate limiting to authenticated endpoints if enabled
	if rateLimitCfg.Enabled && rateLimitStore != nil {
		protected = protected.WithErrorStatuses(http.StatusTooManyRequests)
		protected.Use(middleware.RateLimit(middleware.RateLimitConfig{
			Rate:    rateLimitCfg.API.Authenticated,
			KeyFunc: middleware.UserIDKeyFunc,
			Store:   rateLimitStore,
		}))
	}

	// User, authority, and casbin policy routes - delegated to router groups
	InitializeRouterGroups(protected, db, authCfg)

	// swagger:route GET / general getRoot
	//
	// Root endpoint.
	//
	// Returns basic service information.
	//
	// Produces:
	//   - application/json
	//
	// Responses:
	//   200: rootResponse
	engine.GET("/", func(c *gin.Context) {
		response.Success(c, gin.H{
			"message": fmt.Sprintf("%s is running", appCfg.Name),
		})
	})

	return engine, nil
}
