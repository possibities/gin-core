package pkglogger

import (
	"os"

	"github.com/possibities/gin-boilerplate/pkg/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(cfg *config.Config) (*zap.Logger, func(), error) {
	level := zap.NewAtomicLevel()
	if err := level.UnmarshalText([]byte(cfg.Log.Level)); err != nil {
		level.SetLevel(zap.InfoLevel)
	}

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "timestamp"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewCore(zapcore.NewJSONEncoder(encCfg), zapcore.AddSync(os.Stdout), level)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	zap.ReplaceGlobals(logger)

	cleanup := func() {
		_ = logger.Sync()
	}
	return logger, cleanup, nil
}
