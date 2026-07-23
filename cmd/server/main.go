package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/adapters/out/cache"
	"sprezz/internal/adapters/out/jsonld"
	"sprezz/internal/adapters/out/outbound"
	"sprezz/internal/adapters/out/postgres"
	"sprezz/internal/config"
	"sprezz/internal/domain/ports"
	"sprezz/internal/domain/service"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	log.Println("Starting Sprezz server...")

	tenantConfigPath := config.DefaultTenantConfigPath()
	tenantCfg, err := config.LoadTenantConfig(tenantConfigPath)
	if err != nil {
		log.Printf("Tenant config warning: %v", err)
	}

	// 1. Initialize Ristretto Dictionary Cache (Driven Adapter)
	dictCache, err := cache.NewDictionaryCache()
	if err != nil {
		log.Fatalf("Failed to initialize dictionary cache: %v", err)
	}

	// 2. Initialize Database Connection string securely from Environment Variables
	dbConnStr, err := buildDatabaseConnectionString()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	dbConfig, err := pgxpool.ParseConfig(dbConnStr)
	if err != nil {
		log.Fatalf("Failed to parse postgres configuration: %v", err)
	}
	dbConfig.MaxConns = 25
	dbConfig.MinConns = 10
	dbConfig.MaxConnLifetime = 5 * time.Minute
	db, err := pgxpool.NewWithConfig(context.Background(), dbConfig)
	if err != nil {
		log.Fatalf("Failed to connect to postgres: %v", err)
	}
	defer db.Close()
	if err := db.Ping(context.Background()); err != nil {
		log.Fatalf("Failed to ping postgres: %v", err)
	}

	// 3. Initialize Driven Adapters & Domain Service
	postgresStorage := postgres.NewPostgresStorage(db, dictCache)
	jsonldParser := jsonld.NewJSONLDParser()
	federatedSigner := outbound.NewFederatedSignerAdapter()
	activityService := service.NewActivityService(postgresStorage, jsonldParser, federatedSigner)

	// 4. Start Background Worker Pool for Inbound Tasks
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numWorkers := 4
	for i := 1; i <= numWorkers; i++ {
		go startInboundWorker(ctx, i, postgresStorage, activityService)
	}

	// 5. Setup Driving Adapters (HTTP Router)
	mux := http.NewServeMux()

	// Webfinger resolution handler
	mux.HandleFunc("/.well-known/webfinger", func(w http.ResponseWriter, r *http.Request) {
		if tenantCfg != nil {
			inhttp.HandleWebfingerWithConfig(w, r, tenantConfigPath)
			return
		}
		inhttp.HandleWebfinger(w, r)
	})

	// Inbox handler
	keyResolver := inhttp.NewHTTPPublicKeyResolver(nil)
	inboxHandler := inhttp.NewVerifiedInboxHandler(postgresStorage, inhttp.NewSignatureVerifier(keyResolver))
	mux.Handle("/inbox", inboxHandler)
	mux.Handle("/inbox/", inboxHandler)

	actorHandler := inhttp.NewActorHandler(postgresStorage)
	mux.Handle("/actors/", actorHandler)
	mux.Handle("/actors", actorHandler)

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// 6. Graceful Shutdown Signal Handler
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Sprezz server running on http://localhost:%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down Sprezz server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	cancel() // Stop background workers
	log.Println("Sprezz server stopped gracefully.")
}

func buildDatabaseConnectionString() (string, error) {
	if connStr := os.Getenv("DATABASE_URL"); connStr != "" {
		return connStr, nil
	}

	pgHost := os.Getenv("POSTGRES_HOST")
	pgPort := os.Getenv("POSTGRES_PORT")
	pgUser := os.Getenv("POSTGRES_USER")
	pgPass := os.Getenv("POSTGRES_PASSWORD")
	pgDB := os.Getenv("POSTGRES_DB")
	pgSSL := os.Getenv("POSTGRES_SSLMODE")

	if pgHost == "" {
		pgHost = "localhost"
	}
	if pgPort == "" {
		pgPort = "5432"
	}
	if pgUser == "" {
		pgUser = "sprezz_user"
	}
	if pgDB == "" {
		pgDB = "sprezz_quads"
	}
	if pgSSL == "" {
		pgSSL = "disable"
	}

	if pgPass == "" {
		return "", fmt.Errorf("POSTGRES_PASSWORD or DATABASE_URL environment variable must be specified")
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		pgUser, pgPass, pgHost, pgPort, pgDB, pgSSL), nil
}

func startInboundWorker(ctx context.Context, workerID int, storage ports.StoragePort, svc ports.ActivityServicePort) {
	log.Printf("[Worker %d] Started inbound activity processor", workerID)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Worker %d] Stopping worker", workerID)
			return
		case <-ticker.C:
			tasks, err := storage.ClaimInboundBatch(ctx, 10)
			if err != nil {
				continue
			}
			for _, task := range tasks {
				log.Printf("[Worker %d] Processing task %s (Activity: %s)", workerID, task.ID, task.ActivityIRI)
				if err := svc.ProcessInboundTask(ctx, task); err != nil {
					log.Printf("[Worker %d] Task %s failed: %v", workerID, task.ID, err)
					_ = storage.MarkInboundFailed(ctx, task.ID, err.Error())
				} else {
					log.Printf("[Worker %d] Task %s completed successfully", workerID, task.ID)
					_ = storage.MarkInboundComplete(ctx, task.ID)
				}
			}
		}
	}
}
