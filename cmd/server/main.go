package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/123jjck/avito-trainee-assignment/internal/db"
	"github.com/123jjck/avito-trainee-assignment/internal/service"
	"github.com/123jjck/avito-trainee-assignment/internal/transport/httpserver"
)

func main() {
	ctx := context.Background()
	dsn := getenv("DATABASE_URL", "postgres://pr_service:pr_service@db:5432/pr_service?sslmode=disable")

	sqlDB, err := db.Open(dsn)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer sqlDB.Close()

	if err := waitForDB(ctx, sqlDB); err != nil {
		log.Fatalf("db ping failed: %v", err)
	}
	if err := db.RunMigrations(ctx, sqlDB); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}

	svc := service.New(sqlDB)
	server := httpserver.New(svc)

	port := getenv("PORT", "8080")
	addr := ":" + port
	log.Printf("starting server on %s", addr)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func waitForDB(ctx context.Context, dbConn interface{ PingContext(context.Context) error }) error {
	var lastErr error
	for i := 0; i < 10; i++ {
		if err := dbConn.PingContext(ctx); err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return nil
	}
	return lastErr
}
