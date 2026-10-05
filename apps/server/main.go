package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/certree/certree/internal/app"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func main() {
	health := flag.Bool("healthcheck", false, "check local server health")
	flag.Parse()
	if *health {
		client := http.Client{Timeout: 3 * time.Second}
		r, e := client.Get("http://127.0.0.1:8080/api/system/health")
		if e != nil || r.StatusCode != 200 {
			os.Exit(1)
		}
		r.Body.Close()
		return
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	c := app.Config{Address: env("CERTREE_ADDRESS", "127.0.0.1:8080"), DBPath: env("CERTREE_DB", "data/certree.db"), MasterKey: os.Getenv("CERTREE_MASTER_KEY"), AdminUser: os.Getenv("CERTREE_ADMIN_USER"), AdminPassword: os.Getenv("CERTREE_ADMIN_PASSWORD"), Origin: env("CERTREE_ORIGIN", "https://localhost:8443"), WebDir: env("CERTREE_WEB_DIR", "apps/web/dist"), CAFile: os.Getenv("CERTREE_TRUST_BUNDLE"), InsecureHTTP: os.Getenv("CERTREE_DEV_HTTP") == "true"}
	a, e := app.New(c)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	defer a.Store.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); a.Run(ctx) }()
	server := &http.Server{Addr: c.Address, Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 3 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		slog.Info("Certree started", "address", c.Address)
		if e := server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			slog.Error("listen failed", "error", e)
			cancel()
		}
	}()
	<-ctx.Done()
	shutdown, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	server.Shutdown(shutdown)
	<-workerDone
}
