package adminsystem

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"paigram/internal/service/botroute"
)

type fakeBotRouteAdmin struct {
	listFn   func(ctx context.Context) ([]botroute.BotRouteAdminView, error)
	getFn    func(ctx context.Context, id uint64) (*botroute.BotRouteAdminView, error)
	deleteFn func(ctx context.Context, id uint64) error

	deletedIDs []uint64
}

func (f *fakeBotRouteAdmin) ListBotRoutes(ctx context.Context) ([]botroute.BotRouteAdminView, error) {
	if f.listFn == nil {
		return nil, nil
	}
	return f.listFn(ctx)
}

func (f *fakeBotRouteAdmin) GetBotRouteByID(ctx context.Context, id uint64) (*botroute.BotRouteAdminView, error) {
	if f.getFn == nil {
		return nil, botroute.ErrRouteNotFound
	}
	return f.getFn(ctx, id)
}

func (f *fakeBotRouteAdmin) DeleteBotRoute(ctx context.Context, id uint64) error {
	if f.deleteFn != nil {
		err := f.deleteFn(ctx, id)
		if err != nil {
			return err
		}
	}
	f.deletedIDs = append(f.deletedIDs, id)
	return nil
}

func newBotRouteRouter(t *testing.T, fake *fakeBotRouteAdmin) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewBotRouteHandler(fake)
	r := gin.New()
	r.GET("/bot-routes", h.ListBotRoutes)
	r.GET("/bot-routes/:id", h.GetBotRoute)
	r.DELETE("/bot-routes/:id", h.DeleteBotRoute)
	return r
}

func TestBotRouteHandler_ListReturnsServiceResults(t *testing.T) {
	fake := &fakeBotRouteAdmin{
		listFn: func(ctx context.Context) ([]botroute.BotRouteAdminView, error) {
			return []botroute.BotRouteAdminView{{
				ID:        42,
				BotID:     "paigrambot",
				Platform:  "telegram",
				ServiceID: "paigram-genshin",
				Endpoint:  "paigram-genshin:50052",
				Version:   "1.0.0",
				Handlers:  []botroute.BotRouteHandlerView{{Command: "sign"}},
			}}, nil
		},
	}
	r := newBotRouteRouter(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bot-routes", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Code int                          `json:"code"`
		Data []botroute.BotRouteAdminView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, http.StatusOK, body.Code)
	require.Len(t, body.Data, 1)
	assert.EqualValues(t, 42, body.Data[0].ID)
	assert.Equal(t, "paigram-genshin", body.Data[0].ServiceID)
}

func TestBotRouteHandler_GetReturnsNotFound(t *testing.T) {
	fake := &fakeBotRouteAdmin{
		getFn: func(ctx context.Context, id uint64) (*botroute.BotRouteAdminView, error) {
			return nil, botroute.ErrRouteNotFound
		},
	}
	r := newBotRouteRouter(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bot-routes/99", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestBotRouteHandler_GetRejectsInvalidID(t *testing.T) {
	fake := &fakeBotRouteAdmin{}
	r := newBotRouteRouter(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/bot-routes/not-a-number", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBotRouteHandler_DeleteSucceeds(t *testing.T) {
	fake := &fakeBotRouteAdmin{}
	r := newBotRouteRouter(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/bot-routes/7", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.Len(t, fake.deletedIDs, 1)
	assert.EqualValues(t, 7, fake.deletedIDs[0])
}

func TestBotRouteHandler_DeleteReturnsNotFoundFromService(t *testing.T) {
	fake := &fakeBotRouteAdmin{
		deleteFn: func(ctx context.Context, id uint64) error {
			return botroute.ErrRouteNotFound
		},
	}
	r := newBotRouteRouter(t, fake)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/bot-routes/"+strconv.Itoa(123), nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}
