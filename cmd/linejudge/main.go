package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hiroshi-os/linejudge/internal/fixtures"
	"github.com/hiroshi-os/linejudge/internal/httpapi"
	"github.com/hiroshi-os/linejudge/internal/store"
	"github.com/hiroshi-os/linejudge/internal/worker"
)

func main() {
	addr := flag.String("addr", getenv("LINEJUDGE_ADDR", ":8080"), "http listen address")
	dbPath := flag.String("db", getenv("LINEJUDGE_DB", "data/linejudge.db"), "sqlite path")
	role := flag.String("role", getenv("LINEJUDGE_ROLE", "all"), "all | api | worker")
	flag.Parse()

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	fxDir := fixtures.Dir()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := httpapi.SeedDemoRepos(ctx, st); err != nil {
		log.Fatal(err)
	}

	w := worker.New(st)
	if *role == "all" || *role == "worker" {
		go w.Loop(ctx)
		log.Printf("worker started")
	}

	if *role == "worker" {
		<-ctx.Done()
		return
	}

	srv := httpapi.New(st, fxDir)
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("linejudge api %s  db=%s  fixtures=%s", *addr, *dbPath, fxDir)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	<-ctx.Done()
	shctx, c := context.WithTimeout(context.Background(), 5*time.Second)
	defer c()
	_ = httpSrv.Shutdown(shctx)
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
