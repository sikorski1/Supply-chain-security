package main

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/sikorski1/Supply-chain-security/services/orders/internal"
	repository "github.com/sikorski1/Supply-chain-security/services/orders/internal/adapters/postgresql/sqlc"
)

func (app *application) mount() http.Handler {
	orders := internal.NewHandler(internal.NewService(repository.New(app.db)))
	return newRouter(orders)
}

type ordersAPI interface {
	GetOrders(http.ResponseWriter, *http.Request)
	GetOrder(http.ResponseWriter, *http.Request)
	CreateOrder(http.ResponseWriter, *http.Request)
}

func newRouter(orders ordersAPI) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)              // important for rate limiting
	r.Use(middleware.ClientIPFromRemoteAddr) // important for rate limiting and analytics and tracing
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer) // recovery from crashes

	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("all good"))
	})

	r.Get("/api/orders", orders.GetOrders)
	r.Get("/api/orders/{id}", orders.GetOrder)
	r.Post("/api/orders", orders.CreateOrder)

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
	// logger
	db *pgx.Conn
}

type config struct {
	addr string
	db   dbConfig
}

type dbConfig struct {
	dsn string
}
