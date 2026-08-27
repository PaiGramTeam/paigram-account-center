package platformbinding

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"paigram/internal/model"
	servicecredentials "paigram/internal/service/credentials"
)

func TestListGrantsPaginatesResults(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, db.Create(&owner).Error)
	binding := model.PlatformAccountBinding{
		OwnerUserID:        owner.ID,
		Platform:           "mihomo",
		ExternalAccountKey: ns("cn:grants"),
		PlatformServiceKey: "mihomo",
		DisplayName:        "Grant List",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, db.Create(&binding).Error)

	for _, consumer := range []string{ConsumerPaiGramBot, ConsumerPamgram, "mihomo.sync"} {
		seedConsumerGrant(t, db, model.ConsumerGrant{
			BindingID: binding.ID,
			Consumer:  consumer,
			Status:    model.ConsumerGrantStatusActive,
		}, "mihomo.status.read")
	}

	items, total, err := service.ListGrants(binding.ID, ListParams{Page: 2, PageSize: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, items, 1)

	ownerItems, ownerTotal, err := service.ListGrantsForOwner(owner.ID, binding.ID, ListParams{Page: 1, PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(3), ownerTotal)
	require.Len(t, ownerItems, 2)
}

func TestPlatformBindingServiceGroupWiresGrantInvalidator(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	platformService := &serviceGroupPlatformServiceStub{}

	group := NewServiceGroup(db, platformService)

	require.Same(t, platformService, group.GrantService.invalidator)
}

func TestGrantServiceSupportsRegistryConsumers(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, db.Create(&owner).Error)
	binding := model.PlatformAccountBinding{
		OwnerUserID:        owner.ID,
		Platform:           "mihomo",
		ExternalAccountKey: ns("cn:grant-consumers"),
		PlatformServiceKey: "mihomo",
		DisplayName:        "Grant Consumers",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, db.Create(&binding).Error)

	for _, consumer := range []string{ConsumerPaiGramBot, ConsumerPamgram} {
		grant, created, err := service.UpsertGrant(UpsertGrantInput{
			BindingID: binding.ID,
			Consumer:  consumer,
			GrantedBy: sql.NullInt64{Int64: int64(owner.ID), Valid: true},
			GrantedAt: time.Now().UTC(),
		})
		require.NoError(t, err)
		assert.True(t, created)
		assert.Equal(t, consumer, grant.Consumer)
		assert.Equal(t, model.ConsumerGrantStatusActive, grant.Status)
		assert.False(t, grant.RevokedAt.Valid)
		assert.Equal(t, defaultConsumerActions, grantActionNames(grant.Actions))
	}
}

func TestGrantServiceSupportsRegisteredCredentialConsumer(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	binding := seedGrantServiceBinding(t, db, "cn:registered-consumer")
	require.NoError(t, db.Create(&model.Bot{
		ID:          "paigram",
		DisplayName: "PaiGram",
		Type:        "SERVICE",
		Status:      "ACTIVE",
		OwnerUserID: binding.OwnerUserID,
	}).Error)
	_, err := servicecredentials.NewService(db).Create(servicecredentials.CreateInput{
		ClientID:    "telegram-service",
		BotID:       "paigram",
		DisplayName: "Telegram Service",
		OwnerUserID: binding.OwnerUserID,
		Audiences:   []string{"account-center"},
		Scopes:      []string{"bot.access.issue_ticket"},
	})
	require.NoError(t, err)

	grant, created, err := NewGrantService(db).UpsertGrant(UpsertGrantInput{
		BindingID: binding.ID,
		Consumer:  "telegram-service",
	})

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, "telegram-service", grant.Consumer)
}

func TestGrantServiceRejectsControlPlaneAction(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	binding := seedGrantServiceBinding(t, db, "cn:grant-control-action")

	grant, created, err := service.UpsertGrant(UpsertGrantInput{
		BindingID: binding.ID,
		Consumer:  ConsumerPaiGramBot,
		Actions:   []string{"mihomo.credential.bind"},
	})

	require.ErrorIs(t, err, ErrGrantActionNotAllowed)
	assert.Nil(t, grant)
	assert.False(t, created)
}

func TestGrantServiceNormalizesActionsAndIncrementsVersionOnChange(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	binding := seedGrantServiceBinding(t, db, "cn:grant-action-version")

	grant, created, err := service.UpsertGrant(UpsertGrantInput{
		BindingID: binding.ID,
		Consumer:  ConsumerPaiGramBot,
		Actions:   []string{"mihomo.status.read", "mihomo.profile.read", "mihomo.status.read"},
	})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, uint64(1), grant.TicketVersion)
	require.Equal(t, []string{"mihomo.profile.read", "mihomo.status.read"}, grantActionNames(grant.Actions))

	updated, created, err := service.UpsertGrant(UpsertGrantInput{
		BindingID: binding.ID,
		Consumer:  ConsumerPaiGramBot,
		Actions:   []string{"mihomo.status.read"},
	})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, uint64(2), updated.TicketVersion)
	require.Equal(t, []string{"mihomo.status.read"}, grantActionNames(updated.Actions))
}

func TestGrantServiceUpsertRejectsUnsupportedConsumer(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, db.Create(&owner).Error)
	binding := model.PlatformAccountBinding{
		OwnerUserID:        owner.ID,
		Platform:           "mihomo",
		ExternalAccountKey: ns("cn:grant-unsupported"),
		PlatformServiceKey: "mihomo",
		DisplayName:        "Grant Unsupported",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, db.Create(&binding).Error)

	grant, created, err := service.UpsertGrant(UpsertGrantInput{
		BindingID: binding.ID,
		Consumer:  "unsupported-consumer",
	})
	assert.ErrorIs(t, err, ErrConsumerNotSupported)
	assert.Nil(t, grant)
	assert.False(t, created)
}

func TestPutGrantRejectsWebConsumer(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	binding := seedGrantServiceBinding(t, db, "cn:grant-web-consumer")

	grant, created, err := service.UpsertGrantForOwner(binding.OwnerUserID, UpsertGrantInput{
		BindingID: binding.ID,
		Consumer:  "web",
		GrantedBy: sql.NullInt64{Int64: int64(binding.OwnerUserID), Valid: true},
		GrantedAt: time.Now().UTC(),
	})
	require.ErrorIs(t, err, ErrConsumerNotSupported)
	assert.Nil(t, grant)
	assert.False(t, created)
}

func TestGrantServiceRevokeGrantIsIdempotentWhenGrantDoesNotExist(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, db.Create(&owner).Error)
	binding := model.PlatformAccountBinding{
		OwnerUserID:        owner.ID,
		Platform:           "mihomo",
		ExternalAccountKey: ns("cn:grant-revoke-idempotent"),
		PlatformServiceKey: "mihomo",
		DisplayName:        "Grant Revoke Idempotent",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, db.Create(&binding).Error)

	revokedAt := time.Now().UTC()
	grant, err := service.RevokeGrant(RevokeGrantInput{
		BindingID: binding.ID,
		Consumer:  ConsumerPaiGramBot,
		RevokedAt: revokedAt,
	})
	require.NoError(t, err)
	assert.Equal(t, binding.ID, grant.BindingID)
	assert.Equal(t, ConsumerPaiGramBot, grant.Consumer)
	assert.Equal(t, model.ConsumerGrantStatusRevoked, grant.Status)
	assert.Equal(t, uint64(1), grant.TicketVersion)
	assert.True(t, grant.RevokedAt.Valid)
	assert.WithinDuration(t, revokedAt, grant.RevokedAt.Time, time.Microsecond)
	assert.True(t, grant.LastInvalidatedAt.Valid)
	assert.WithinDuration(t, revokedAt, grant.LastInvalidatedAt.Time, time.Microsecond)

	var count int64
	require.NoError(t, db.Model(&model.ConsumerGrant{}).Where("binding_id = ? AND consumer = ?", binding.ID, ConsumerPaiGramBot).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestGrantServiceRevokeGrantIncrementsTicketVersion(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	binding := seedGrantServiceBinding(t, db, "cn:grant-version")
	revokedAt := time.Now().UTC()
	seedConsumerGrant(t, db, model.ConsumerGrant{
		BindingID:     binding.ID,
		Consumer:      ConsumerPaiGramBot,
		Status:        model.ConsumerGrantStatusActive,
		TicketVersion: 1,
		GrantedAt:     time.Now().UTC(),
	}, "mihomo.status.read")

	revoked, err := service.RevokeGrant(RevokeGrantInput{
		BindingID: binding.ID,
		Consumer:  ConsumerPaiGramBot,
		RevokedAt: revokedAt,
	})

	require.NoError(t, err)
	assert.Equal(t, model.ConsumerGrantStatusRevoked, revoked.Status)
	assert.Equal(t, uint64(2), revoked.TicketVersion)
	assert.True(t, revoked.RevokedAt.Valid)
	assert.True(t, revoked.LastInvalidatedAt.Valid)
	assert.WithinDuration(t, revokedAt, revoked.LastInvalidatedAt.Time, time.Millisecond)
}

func TestGrantServiceRevokeGrantAlreadyRevokedDoesNotIncrementTicketVersion(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	binding := seedGrantServiceBinding(t, db, "cn:grant-already-revoked")
	revokedAt := time.Now().UTC().Add(-time.Hour)
	seedConsumerGrant(t, db, model.ConsumerGrant{
		BindingID:         binding.ID,
		Consumer:          ConsumerPaiGramBot,
		Status:            model.ConsumerGrantStatusRevoked,
		TicketVersion:     3,
		GrantedAt:         time.Now().UTC(),
		RevokedAt:         sql.NullTime{Time: revokedAt, Valid: true},
		LastInvalidatedAt: sql.NullTime{Time: revokedAt, Valid: true},
	})

	revoked, err := service.RevokeGrant(RevokeGrantInput{
		BindingID: binding.ID,
		Consumer:  ConsumerPaiGramBot,
		RevokedAt: time.Now().UTC(),
	})

	require.NoError(t, err)
	assert.Equal(t, model.ConsumerGrantStatusRevoked, revoked.Status)
	assert.Equal(t, uint64(3), revoked.TicketVersion)
	assert.True(t, revoked.LastInvalidatedAt.Valid)
	assert.WithinDuration(t, revokedAt, revoked.LastInvalidatedAt.Time, time.Millisecond)
}

func TestGrantServiceUpsertGrantReactivationIncrementsTicketVersion(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	binding := seedGrantServiceBinding(t, db, "cn:grant-reactivate")
	seedConsumerGrant(t, db, model.ConsumerGrant{
		BindingID:         binding.ID,
		Consumer:          ConsumerPaiGramBot,
		Status:            model.ConsumerGrantStatusRevoked,
		TicketVersion:     4,
		GrantedAt:         time.Now().UTC(),
		RevokedAt:         sql.NullTime{Time: time.Now().UTC(), Valid: true},
		LastInvalidatedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
	}, defaultConsumerActions...)

	grant, created, err := service.UpsertGrant(UpsertGrantInput{
		BindingID: binding.ID,
		Consumer:  ConsumerPaiGramBot,
		GrantedBy: sql.NullInt64{Int64: int64(binding.OwnerUserID), Valid: true},
		GrantedAt: time.Now().UTC(),
	})

	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, model.ConsumerGrantStatusActive, grant.Status)
	assert.False(t, grant.RevokedAt.Valid)
	assert.Equal(t, uint64(5), grant.TicketVersion)
}

func TestGrantServiceUpsertActionContractionPropagatesMinimumTicketVersion(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	invalidator := &capturingGrantInvalidator{}
	service := NewGrantService(db, invalidator)
	binding := seedGrantServiceBinding(t, db, "cn:grant-action-contraction")
	seedConsumerGrant(t, db, model.ConsumerGrant{
		BindingID: binding.ID, Consumer: ConsumerPaiGramBot, Status: model.ConsumerGrantStatusActive,
		TicketVersion: 3, GrantedAt: time.Now().UTC(),
	}, "mihomo.status.read", "mihomo.profile.read")
	ctx := context.WithValue(context.Background(), grantInvalidatorContextKey{}, "action-contraction")

	grant, created, err := service.UpsertGrant(UpsertGrantInput{
		Context: ctx, BindingID: binding.ID, Consumer: ConsumerPaiGramBot,
		Actions: []string{"mihomo.status.read"}, GrantedBy: sql.NullInt64{Int64: int64(binding.OwnerUserID), Valid: true},
	})

	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, uint64(4), grant.TicketVersion)
	require.True(t, grant.LastInvalidatedAt.Valid)
	require.Equal(t, 1, invalidator.calls)
	require.Equal(t, ctx, invalidator.ctx)
	require.Equal(t, uint64(4), invalidator.input.MinimumGrantVersion)
	require.Equal(t, ConsumerPaiGramBot, invalidator.input.Consumer)
}

func TestGrantServiceUpsertRetriesPendingActionInvalidation(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	binding := seedGrantServiceBinding(t, db, "cn:grant-action-retry")
	seedConsumerGrant(t, db, model.ConsumerGrant{
		BindingID: binding.ID, Consumer: ConsumerPaiGramBot, Status: model.ConsumerGrantStatusActive,
		TicketVersion: 4, GrantedAt: time.Now().UTC(), LastInvalidatedAt: sql.NullTime{},
	}, "mihomo.status.read")
	invalidator := &capturingGrantInvalidator{}
	service := NewGrantService(db, invalidator)

	grant, created, err := service.UpsertGrant(UpsertGrantInput{
		BindingID: binding.ID, Consumer: ConsumerPaiGramBot, Actions: []string{"mihomo.status.read"},
		GrantedBy: sql.NullInt64{Int64: int64(binding.OwnerUserID), Valid: true},
	})

	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, 1, invalidator.calls)
	require.Equal(t, uint64(4), invalidator.input.MinimumGrantVersion)
	require.True(t, grant.LastInvalidatedAt.Valid)
}

func TestGrantServiceRevokeGrantInvalidatorFailureLeavesRetryableRevocation(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	invalidationErr := errors.New("platform down")
	service := NewGrantService(db, failingGrantInvalidator{err: invalidationErr})
	binding := seedGrantServiceBinding(t, db, "cn:grant-invalid-failure")
	seedConsumerGrant(t, db, model.ConsumerGrant{
		BindingID:     binding.ID,
		Consumer:      ConsumerPaiGramBot,
		Status:        model.ConsumerGrantStatusActive,
		TicketVersion: 2,
		GrantedAt:     time.Now().UTC(),
	}, "mihomo.status.read")

	grant, err := service.RevokeGrant(RevokeGrantInput{
		Context:     context.Background(),
		BindingID:   binding.ID,
		Consumer:    ConsumerPaiGramBot,
		RevokedAt:   time.Now().UTC(),
		ActorUserID: sql.NullInt64{Int64: int64(binding.OwnerUserID), Valid: true},
	})

	require.ErrorIs(t, err, invalidationErr)
	assert.Nil(t, grant)

	var stored model.ConsumerGrant
	require.NoError(t, db.Where("binding_id = ? AND consumer = ?", binding.ID, ConsumerPaiGramBot).First(&stored).Error)
	assert.Equal(t, model.ConsumerGrantStatusRevoked, stored.Status)
	assert.True(t, stored.RevokedAt.Valid)
	assert.Equal(t, uint64(3), stored.TicketVersion)
	assert.False(t, stored.LastInvalidatedAt.Valid)
}

func TestGrantServiceRevokeGrantRetriesMissingInvalidationForRevokedGrant(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	invalidator := &capturingGrantInvalidator{}
	service := NewGrantService(db, invalidator)
	binding := seedGrantServiceBinding(t, db, "cn:grant-retry-invalid")
	revokedAt := time.Now().UTC().Add(-time.Hour)
	retryAt := time.Now().UTC()
	seedConsumerGrant(t, db, model.ConsumerGrant{
		BindingID:     binding.ID,
		Consumer:      ConsumerPaiGramBot,
		Status:        model.ConsumerGrantStatusRevoked,
		TicketVersion: 5,
		GrantedAt:     time.Now().UTC(),
		RevokedAt:     sql.NullTime{Time: revokedAt, Valid: true},
	})

	revoked, err := service.RevokeGrant(RevokeGrantInput{
		BindingID: binding.ID,
		Consumer:  ConsumerPaiGramBot,
		RevokedAt: retryAt,
	})

	require.NoError(t, err)
	require.Equal(t, 1, invalidator.calls)
	assert.Equal(t, uint64(5), invalidator.input.MinimumGrantVersion)
	assert.Equal(t, uint64(5), revoked.TicketVersion)
	assert.True(t, revoked.LastInvalidatedAt.Valid)
	assert.WithinDuration(t, retryAt, revoked.LastInvalidatedAt.Time, time.Millisecond)
}

func TestGrantInvalidationCompletionDoesNotConfirmNewerPendingVersion(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	binding := seedGrantServiceBinding(t, db, "cn:grant-newer-pending")
	current := seedConsumerGrant(t, db, model.ConsumerGrant{
		BindingID: binding.ID, Consumer: ConsumerPaiGramBot, Status: model.ConsumerGrantStatusRevoked,
		TicketVersion: 3, GrantedAt: time.Now().UTC(), RevokedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
	})
	stale := current
	stale.TicketVersion = 2

	err := service.completeGrantInvalidation(&stale, 2, 0, time.Now().UTC())
	require.ErrorIs(t, err, ErrGrantPropagationPending)
	var stored model.ConsumerGrant
	require.NoError(t, db.First(&stored, current.ID).Error)
	assert.False(t, stored.LastInvalidatedAt.Valid)
}

func TestGrantServiceRevokeGrantCallsInvalidatorWithExpectedInput(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	invalidator := &capturingGrantInvalidator{}
	service := NewGrantService(db, invalidator)
	binding := seedGrantServiceBinding(t, db, "cn:grant-invalid-input")
	seedConsumerGrant(t, db, model.ConsumerGrant{
		BindingID:     binding.ID,
		Consumer:      ConsumerPaiGramBot,
		Status:        model.ConsumerGrantStatusActive,
		TicketVersion: 7,
		GrantedAt:     time.Now().UTC(),
	}, "mihomo.status.read")
	ctx := context.WithValue(context.Background(), grantInvalidatorContextKey{}, "request-context")

	_, err := service.RevokeGrant(RevokeGrantInput{
		Context:     ctx,
		BindingID:   binding.ID,
		Consumer:    ConsumerPaiGramBot,
		RevokedAt:   time.Now().UTC(),
		ActorUserID: sql.NullInt64{Int64: int64(binding.OwnerUserID), Valid: true},
	})

	require.NoError(t, err)
	require.Equal(t, 1, invalidator.calls)
	assert.Same(t, ctx, invalidator.ctx)
	assert.Equal(t, GrantInvalidationInput{
		BindingID:           binding.ID,
		BindingRef:          binding.BindingRef,
		OwnerUserID:         binding.OwnerUserID,
		Platform:            "mihomo",
		PlatformServiceKey:  "mihomo",
		Consumer:            ConsumerPaiGramBot,
		MinimumGrantVersion: 8,
		ActorType:           "user",
		ActorID:             strconv.FormatUint(binding.OwnerUserID, 10),
	}, invalidator.input)
}

func TestGrantServiceRevokeGrantAuditFailureIsBestEffort(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	binding := seedGrantServiceBinding(t, db, "cn:grant-audit-best-effort")
	seedConsumerGrant(t, db, model.ConsumerGrant{
		BindingID:     binding.ID,
		Consumer:      ConsumerPaiGramBot,
		Status:        model.ConsumerGrantStatusActive,
		TicketVersion: 1,
		GrantedAt:     time.Now().UTC(),
	}, "mihomo.status.read")
	require.NoError(t, db.Exec(`
		CREATE FUNCTION fail_audit_event_insert() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'audit disabled';
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER audit_events_fail_before_insert
		BEFORE INSERT ON audit_events
		FOR EACH ROW EXECUTE FUNCTION fail_audit_event_insert();
	`).Error)

	grant, err := service.RevokeGrant(RevokeGrantInput{
		BindingID: binding.ID,
		Consumer:  ConsumerPaiGramBot,
		RevokedAt: time.Now().UTC(),
	})

	require.NoError(t, err)
	require.NotNil(t, grant)
	assert.Equal(t, model.ConsumerGrantStatusRevoked, grant.Status)
	assert.Equal(t, uint64(2), grant.TicketVersion)

	var stored model.ConsumerGrant
	require.NoError(t, db.Where("binding_id = ? AND consumer = ?", binding.ID, ConsumerPaiGramBot).First(&stored).Error)
	assert.Equal(t, model.ConsumerGrantStatusRevoked, stored.Status)
	assert.Equal(t, uint64(2), stored.TicketVersion)
}

func TestGrantServiceUpsertWritesUnifiedAuditEvent(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	service := NewGrantService(db)
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, db.Create(&owner).Error)
	binding := model.PlatformAccountBinding{
		OwnerUserID:        owner.ID,
		Platform:           "mihomo",
		ExternalAccountKey: ns("cn:grant-audit"),
		PlatformServiceKey: "mihomo",
		DisplayName:        "Grant Audit",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, db.Create(&binding).Error)

	_, _, err := service.UpsertGrant(UpsertGrantInput{
		BindingID: binding.ID,
		Consumer:  ConsumerPaiGramBot,
		GrantedBy: sql.NullInt64{Int64: int64(owner.ID), Valid: true},
		GrantedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	var event model.AuditEvent
	require.NoError(t, db.Where("category = ? AND action = ?", "platform_binding", "grant_change").Order("id DESC").First(&event).Error)
	assert.Equal(t, "binding", event.TargetType)
	assert.Equal(t, "success", event.Result)
	assert.Equal(t, int64(binding.ID), event.BindingID.Int64)
	metadata := requireGrantAuditMetadata(t, event.MetadataJSON)
	assert.Equal(t, ConsumerPaiGramBot, metadata["consumer"])
	assert.Equal(t, true, metadata["grant_enabled"])
}

func TestGrantServiceRevokeWritesAdminActorAttribution(t *testing.T) {
	db := setupPlatformBindingTestDB(t)
	invalidator := &capturingGrantInvalidator{}
	service := NewGrantService(db, invalidator)
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	admin := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&admin).Error)
	binding := model.PlatformAccountBinding{
		OwnerUserID:        owner.ID,
		Platform:           "mihomo",
		ExternalAccountKey: ns("cn:grant-revoke-audit"),
		PlatformServiceKey: "mihomo",
		DisplayName:        "Grant Revoke Audit",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, db.Create(&binding).Error)
	seedConsumerGrant(t, db, model.ConsumerGrant{
		BindingID: binding.ID,
		Consumer:  ConsumerPaiGramBot,
		Status:    model.ConsumerGrantStatusActive,
		GrantedBy: sql.NullInt64{Int64: int64(owner.ID), Valid: true},
		GrantedAt: time.Now().UTC(),
	}, "mihomo.status.read")

	_, err := service.RevokeGrant(RevokeGrantInput{
		BindingID:   binding.ID,
		Consumer:    ConsumerPaiGramBot,
		RevokedAt:   time.Now().UTC(),
		ActorUserID: sql.NullInt64{Int64: int64(admin.ID), Valid: true},
	})
	require.NoError(t, err)
	require.Equal(t, 1, invalidator.calls)
	assert.Equal(t, "admin", invalidator.input.ActorType)
	assert.Equal(t, strconv.FormatUint(admin.ID, 10), invalidator.input.ActorID)

	var event model.AuditEvent
	require.NoError(t, db.Where("category = ? AND action = ?", "platform_binding", "grant_change").Order("id DESC").First(&event).Error)
	assert.Equal(t, "admin", event.ActorType)
	assert.True(t, event.ActorUserID.Valid)
	assert.Equal(t, int64(admin.ID), event.ActorUserID.Int64)
	metadata := requireGrantAuditMetadata(t, event.MetadataJSON)
	assert.Equal(t, ConsumerPaiGramBot, metadata["consumer"])
	assert.Equal(t, false, metadata["grant_enabled"])
	actor, ok := metadata["actor"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "admin", actor["type"])
}

func requireGrantAuditMetadata(t *testing.T, metadataJSON string) map[string]any {
	t.Helper()
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(metadataJSON), &metadata))
	return metadata
}

func seedGrantServiceBinding(t *testing.T, db *gorm.DB, externalAccountKey string) model.PlatformAccountBinding {
	t.Helper()
	owner := model.User{PrimaryLoginType: model.LoginTypeEmail, Status: model.UserStatusActive}
	require.NoError(t, db.Create(&owner).Error)
	binding := model.PlatformAccountBinding{
		OwnerUserID:        owner.ID,
		Platform:           "mihomo",
		ExternalAccountKey: ns(externalAccountKey),
		PlatformServiceKey: "mihomo",
		DisplayName:        "Grant Service",
		Status:             model.PlatformAccountBindingStatusActive,
	}
	require.NoError(t, db.Create(&binding).Error)
	return binding
}

func seedConsumerGrant(t *testing.T, db *gorm.DB, grant model.ConsumerGrant, actions ...string) model.ConsumerGrant {
	t.Helper()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Actions").Create(&grant).Error; err != nil {
			return err
		}
		rows := make([]model.ConsumerGrantAction, 0, len(actions))
		for _, action := range actions {
			rows = append(rows, model.ConsumerGrantAction{GrantID: grant.ID, Action: action})
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	}))
	grant.Actions = make([]model.ConsumerGrantAction, 0, len(actions))
	for _, action := range actions {
		grant.Actions = append(grant.Actions, model.ConsumerGrantAction{GrantID: grant.ID, Action: action})
	}
	return grant
}

type failingGrantInvalidator struct {
	err error
}

func (f failingGrantInvalidator) InvalidateConsumerGrant(context.Context, GrantInvalidationInput) error {
	return f.err
}

type grantInvalidatorContextKey struct{}

type capturingGrantInvalidator struct {
	calls int
	ctx   context.Context
	input GrantInvalidationInput
}

func (c *capturingGrantInvalidator) InvalidateConsumerGrant(ctx context.Context, input GrantInvalidationInput) error {
	c.calls++
	c.ctx = ctx
	c.input = input
	return nil
}

type serviceGroupPlatformServiceStub struct{}

func (s *serviceGroupPlatformServiceStub) GetEnabledPlatform(string) (*model.PlatformService, error) {
	return &model.PlatformService{ServiceKey: "platform-mihomo-service", ServiceAudience: "platform-mihomo-service"}, nil
}

func (s *serviceGroupPlatformServiceStub) IssueBindingScopedTicket(string, string, *model.PlatformAccountBinding, []string) (string, time.Time, error) {
	return "ticket", time.Now().UTC(), nil
}

func (s *serviceGroupPlatformServiceStub) IssueBindingScopedOperationTicket(string, string, *model.PlatformAccountBinding, string, []string) (string, time.Time, error) {
	return "ticket", time.Now().UTC(), nil
}

func (s *serviceGroupPlatformServiceStub) IssueProfileScopedOperationTicket(string, string, *model.PlatformAccountBinding, string, string, []string) (string, time.Time, error) {
	return "ticket", time.Now().UTC(), nil
}

func (s *serviceGroupPlatformServiceStub) GetBindingRuntimeSummary(context.Context, string, string, *model.PlatformAccountBinding, []string) (map[string]any, error) {
	return nil, nil
}

func (s *serviceGroupPlatformServiceStub) InvalidateConsumerGrant(context.Context, GrantInvalidationInput) error {
	return nil
}
