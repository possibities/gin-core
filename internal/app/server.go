package app

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/possibities/gin-boilerplate/pkg/config"
)

func NewHTTPServer(cfg *config.Config, engine *gin.Engine) *http.Server {
	addr := fmt.Sprintf("%s:%d", cfg.App.Host, cfg.App.Port)
	readTimeout := time.Duration(cfg.App.ReadTimeoutSec) * time.Second
	writeTimeout := time.Duration(cfg.App.WriteTimeoutSec) * time.Second
	return &http.Server{
		Addr:         addr,
		Handler:      engine,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}
}
