package main

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/sikorski1/Supply-chain-security/services/inventory/internal"
	"github.com/sikorski1/Supply-chain-security/services/inventory/internal/adapters/postgresql"
)

func (app *application) mount() http.Handler {
	inventory := internal.NewHandler(internal.NewService(postgresql.New(app.db)))
	return newRouter(inventory)
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

func (app *application) run(h http.Handler) error {
	srv := &http.Server{
		Addr:         app.config.addr,
		Handler:      h,
		WriteTimeout: time.Second * 30,
		ReadTimeout:  time.Second * 10,
		IdleTimeout:  time.Minute,
	}
	log.Printf("Server has started at addr %s", app.config.addr)
	return srv.ListenAndServe()
}

type application struct {
	config config
	db     *pgx.Conn
}

type config struct {
	addr string
	db   dbConfig
}

type dbConfig struct {
	dsn string
}
