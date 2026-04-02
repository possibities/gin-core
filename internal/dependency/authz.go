package dependency

import (
	"time"

	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/possibities/gin-boilerplate/pkg/config"
	"gorm.io/gorm"
)

const casbinModel = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && keyMatch2(r.obj, p.obj) && regexMatch(r.act, p.act)
`

func NewRBACEnforcer(cfg *config.Config, db *gorm.DB) (*casbin.SyncedEnforcer, func(), error) {
	model, err := casbinmodel.NewModelFromString(casbinModel)
	if err != nil {
		return nil, nil, err
	}

	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		return nil, nil, err
	}

	enforcer, err := casbin.NewSyncedEnforcer(model, adapter)
	if err != nil {
		return nil, nil, err
	}
	if err := enforcer.LoadPolicy(); err != nil {
		return nil, nil, err
	}

	cleanup := func() {}
	if cfg.RBAC.Enabled {
		interval := time.Duration(cfg.RBAC.AutoLoadIntervalSec) * time.Second
		enforcer.StartAutoLoadPolicy(interval)
		cleanup = func() {
			enforcer.StopAutoLoadPolicy()
		}
	}

	return enforcer, cleanup, nil
}
