// Weekset API. This process serves health probes until the feature
// modules are wired.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"

	httpadapter "github.com/vickydk/weekset-api/internal/adapter/http"
	"github.com/vickydk/weekset-api/internal/platform/logger"
)

func main() {
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	err := httpadapter.NewRouter().Start(":" + port)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Get().Error(context.Background(), "server stopped", logger.Field{Key: "error", Val: err})
		os.Exit(1)
	}
}
