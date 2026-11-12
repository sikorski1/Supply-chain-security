package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sikorski1/Supply-chain-security/services/inventory/internal"
	"github.com/sikorski1/Supply-chain-security/services/inventory/internal/adapters/postgresql"
)

func (app *application) mount() http.Handler {
	inventory := internal.NewHandler(internal.NewService(postgresql.New(app.db)))
	router := newRouter(inventory)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" && r.Method == http.MethodGet {
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			if err := app.db.Ping(ctx); err != nil {
				http.Error(w, "database unavailable", http.StatusServiceUnavailable)
				return
			}
		}
		router.ServeHTTP(w, r)
	})
}

type inventoryAPI interface {
	GetProducts(http.ResponseWriter, *http.Request)
	GetProduct(http.ResponseWriter, *http.Request)
	CreateReservation(http.ResponseWriter, *http.Request)
	ReleaseReservation(http.ResponseWriter, *http.Request)
}

func newRouter(inventory inventoryAPI) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)              // important for rate limiting
	r.Use(middleware.ClientIPFromRemoteAddr) // important for rate limiting and analytics and tracing
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer) // recovery from crashes

	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("all good"))
	})

	r.Get("/api/products", inventory.GetProducts)
	r.Get("/internal/products/{id}", inventory.GetProduct)
	r.Post("/internal/reservations", inventory.CreateReservation)
	r.Post("/internal/reservations/{orderId}/release", inventory.ReleaseReservation)

	return r
}

func (app *application) run(ctx context.Context, h http.Handler) error {
	srv := &http.Server{
		Addr:         app.config.addr,
		Handler:      h,
		WriteTimeout: time.Second * 30,
		ReadTimeout:  time.Second * 10,
		IdleTimeout:  time.Minute,
	}
	log.Printf("Server has started at addr %s", app.config.addr)
	done := make(chan struct{})
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-ctx.Done():
		case <-done:
			return
		}
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			_ = srv.Close()
		}
	}()
	err := srv.ListenAndServe()
	close(done)
	<-shutdownDone
	return err
}

type application struct {
	config config
	db     *pgxpool.Pool
}

type config struct {
	addr string
	db   dbConfig
}

type dbConfig struct {
	dsn string
}
