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

	// 5. Start Symmetrical Background Worker Engines (Inbound & Outbound)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Inbound Worker Configuration Integration
	inboundEngine := service.NewInboundWorkerEngine(service.WorkerConfig{
		NumWorkers: 4,
		BatchSize:  10,
		PollDelay:  500 * time.Millisecond,
	}, postgresStorage, activityService)

	go func() {
		log.Println("Launching async Inbound Worker Engine...")
		if err := inboundEngine.Start(ctx); err != nil {
			log.Printf("Inbound worker engine exited with error: %v", err)
		}
	}()

	// Outbound Federation Worker Configuration Integration
	outboundEngine := service.NewOutboundWorkerEngine(service.OutboundWorkerConfig{
		NumWorkers: 4,
		BatchSize:  10,
		PollDelay:  500 * time.Millisecond,
	}, postgresStorage, federatedSigner)

	go func() {
		log.Println("Launching async Outbound Federation Worker Engine...")
		if err := outboundEngine.Start(ctx); err != nil {
			log.Printf("Outbound worker engine exited with error: %v", err)
		}
	}()

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

	cancel() // Stop all background worker engines symmetrically via context cancellation
	log.Println("Sprezz server stopped gracefully.")
}
