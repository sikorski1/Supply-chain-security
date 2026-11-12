package main

import (
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sikorski1/Supply-chain-security/api-gateway/internal"
)

func (app *application) mount() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.ClientIPFromRemoteAddr)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	h := internal.NewHandler(app.orders, app.inventory)

	r.Get("/health", h.Health)

	r.Get("/api/products", h.Products)

	r.Get("/api/orders", h.Orders)
	r.Get("/api/orders/{id}", h.Orders)
	r.Post("/api/orders", h.Orders)

	r.NotFound(h.NotFound)
	r.MethodNotAllowed(h.MethodNotAllowed)

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
	config    config
	orders    http.Handler
	inventory http.Handler
}

type config struct {
	addr                string
	ordersServiceURL    string
	inventoryServiceURL string
	upstreamTimeout     time.Duration
}
