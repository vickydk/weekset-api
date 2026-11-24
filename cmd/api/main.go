// Weekset API. This process serves health probes until the feature
// modules are wired.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	httpadapter "github.com/vickydk/weekset-api/internal/adapter/http"
	"github.com/vickydk/weekset-api/internal/adapter/postgres"
	"github.com/vickydk/weekset-api/internal/platform/logger"
	"github.com/vickydk/weekset-api/internal/platform/password"
	"github.com/vickydk/weekset-api/internal/platform/token"
	authuc "github.com/vickydk/weekset-api/internal/usecase/auth"
	centeruc "github.com/vickydk/weekset-api/internal/usecase/center"
	peopleuc "github.com/vickydk/weekset-api/internal/usecase/people"
)

func main() {
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	router := httpadapter.NewRouter()
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		store, err := postgres.NewStore(context.Background(), databaseURL)
		if err != nil {
			logger.Get().Error(context.Background(), "database", logger.Field{Key: "error", Val: err})
			os.Exit(1)
		}
		defer store.Close()
		secret := os.Getenv("JWT_SECRET")
		if secret == "" {
			secret = "development-only-secret"
		}
		authSvc := &authuc.Service{
			Users:  store,
			Tokens: store,
			Resets: store,
			Mailer: postgres.LogMailer{},
			Hasher: password.Default(),
			Issuer: token.Issuer{Secret: []byte(secret), TTL: 15 * time.Minute},
		}
		httpadapter.MountAuth(router, authSvc)
		peopleSvc := &peopleuc.Service{Centers: store, Users: store}
		httpadapter.MountCenter(router, &centeruc.Service{
			Centers: store,
			Users:   store,
			Auth:    authSvc,
			Hasher:  password.Default(),
		}, peopleSvc, authSvc)
		httpadapter.MountPeople(router, peopleSvc, authSvc)
	}

	err := router.Start(":" + port)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Get().Error(context.Background(), "server stopped", logger.Field{Key: "error", Val: err})
		os.Exit(1)
	}
}
