package casbin

import (
	"sync"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"gorm.io/gorm"
)

var (
	enforcer *casbin.SyncedCachedEnforcer
	once     sync.Once
	mu       sync.RWMutex // protects enforcer and once
)

func InitEnforcer(db *gorm.DB) (*casbin.SyncedCachedEnforcer, error) {
	mu.Lock()
	defer mu.Unlock()

	var err error
	once.Do(func() {
		adapter, adapterErr := gormadapter.NewAdapterByDB(db)
		if adapterErr != nil {
			err = adapterErr
			return
		}

		m := getModelConfig()

		enforcer, err = casbin.NewSyncedCachedEnforcer(m, adapter)
		if err != nil {
			return
		}

		enforcer.SetExpireTime(60 * 60)

		err = enforcer.LoadPolicy()
	})

	if err != nil {
		resetUnlocked()
		return nil, err
	}

	return enforcer, nil
}

func resetUnlocked() {
	enforcer = nil
	once = sync.Once{}
}

// WARNING: Do not call Reset concurrently in production.
// This function is NOT safe for use in production with concurrent InitEnforcer calls
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	resetUnlocked()
}

// Panics if InitEnforcer() hasn't been called yet
func GetEnforcer() *casbin.SyncedCachedEnforcer {
	mu.RLock()
	defer mu.RUnlock()
	if enforcer == nil {
		panic("casbin: GetEnforcer called before InitEnforcer")
	}
	return enforcer
}

func getModelConfig() model.Model {
	m := model.NewModel()
	m.AddDef("r", "r", "sub, obj, act")
	m.AddDef("p", "p", "sub, obj, act")
	m.AddDef("g", "g", "_, _")
	m.AddDef("e", "e", "some(where (p.eft == allow))")
	m.AddDef("m", "m", "g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && r.act == p.act")
	return m
}
