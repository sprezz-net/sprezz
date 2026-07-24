package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	inhttp "sprezz/internal/adapters/in/http"
	"sprezz/internal/adapters/out/cache"
	"sprezz/internal/adapters/out/jsonld"
	"sprezz/internal/adapters/out/minio"
	"sprezz/internal/adapters/out/outbound"
	"sprezz/internal/adapters/out/postgres"
	"sprezz/internal/config"
	"sprezz/internal/domain/ports"
	"sprezz/internal/domain/service"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	log.Println("Starting Sprezz server...")

	// 1. Initialize CleanEnv Application Configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Configuration bootstrap error: %v", err)
	}

	// 2. Initialize Ristretto Dictionary Cache (Driven Adapter)
	dictCache, err := cache.NewDictionaryCache()
	if err != nil {
		log.Fatalf("Failed to initialize dictionary cache: %v", err)
	}

	// 3. Connect to Database using CleanEnv helper DSN string
	dbConfig, err := pgxpool.ParseConfig(cfg.GetDSN())
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

	// Initialize MinIO (Driven Adapter) and safeguard against startup race conditions
	minioStorage, err := minio.NewMinIOStorageAdapter(
		cfg.MinIO.Endpoint,
		cfg.MinIO.RootUser,
		cfg.MinIO.RootPassword,
		cfg.MinIO.BucketName,
		cfg.MinIO.UseSSL,
	)
	if err != nil {
		log.Fatalf("Critical storage adapter initialization error: %v", err)
	}
	_ = minioStorage // Keeps compile safe if not immediately used in application initialization pipelines below

	// 4. Initialize Driven Adapters & Domain Service
	postgresStorage := postgres.NewPostgresStorage(db, dictCache)
	jsonldParser := jsonld.NewJSONLDParser()
	federatedSigner := outbound.NewFederatedSignerAdapter()
	activityService := service.NewActivityService(postgresStorage, jsonldParser, federatedSigner)

	// 5. Start Background Worker Pool for Inbound Tasks
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numWorkers := 4
	for i := 1; i <= numWorkers; i++ {
		go startInboundWorker(ctx, i, postgresStorage, activityService)
	}

	// 6. Setup Driving Adapters (HTTP Router)
	mux := http.NewServeMux()

	// Pass the pre-loaded config slice right into the driving adapter function
	mux.HandleFunc("/.well-known/webfinger", inhttp.HandleWebfinger(cfg.TenantDomains))

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

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// 7. Graceful Shutdown Signal Handler
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Sprezz server running on http://localhost:%s for domains: %v", cfg.Port, cfg.TenantDomains)
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
