package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sikorski1/Supply-chain-security/services/orders/internal"
	"github.com/sikorski1/Supply-chain-security/services/orders/internal/adapters/postgresql"
	repository "github.com/sikorski1/Supply-chain-security/services/orders/internal/adapters/postgresql/sqlc"
)

func (app *application) mount() http.Handler {
	orders := internal.NewHandler(internal.NewWorkflow(internal.NewService(repository.New(app.db)), postgresql.New(app.db), app.inventoryURL, app.paymentsURL, app.publisher))
	router := newRouter(orders)
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

type ordersAPI interface {
	GetOrders(http.ResponseWriter, *http.Request)
	GetOrder(http.ResponseWriter, *http.Request)
	GetStats(http.ResponseWriter, *http.Request)
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
	r.Get("/api/stats", orders.GetStats)
	r.Post("/api/orders", orders.CreateOrder)

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
	config       config
	publisher    internal.Publisher
	inventoryURL string
	paymentsURL  string
	// logger
	db *pgxpool.Pool
}

type config struct {
	addr string
	db   dbConfig
}

type dbConfig struct {
	dsn string
}
