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
	"sprezz/internal/adapters/in/http/middleware"
	"sprezz/internal/adapters/out/cache"
	"sprezz/internal/adapters/out/jsonld"
	"sprezz/internal/adapters/out/minio"
	"sprezz/internal/adapters/out/outbound"
	"sprezz/internal/adapters/out/postgres"
	"sprezz/internal/config"
	"sprezz/internal/domain/service"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
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

	// Initialize MinIO (Driven Adapter)
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
	_ = minioStorage

	// 4. Initialize Driven Adapters & Domain Service Layers
	postgresStorage := postgres.NewPostgresStorage(db, dictCache)
	jsonldParser := jsonld.NewJSONLDParser()
	federatedSigner := outbound.NewFederatedSignerAdapter()
	activityService := service.NewActivityService(postgresStorage, jsonldParser, federatedSigner)

	// 5. Start Background Batch Worker Engines (Inbound & Outbound)
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

	// 6. Setup Driving Adapters with Chi Router
	r := chi.NewRouter()

	// Inject core optimized infrastructure middleware globally
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.Timeout(60 * time.Second))

	// Global baseline health endpoints (accessible without tenant locks)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Setup Inbound Activity Handlers
	keyResolver := inhttp.NewHTTPPublicKeyResolver(nil)
	sigVerifier := inhttp.NewSignatureVerifier(keyResolver)
	inboxHandler := inhttp.NewInboxHandler(postgresStorage)
	actorHandler := inhttp.NewActorHandler(postgresStorage)

	// Instantiate the tenant validator from middleware package
	tenantValidator := middleware.NewTenantValidator(middleware.TenantConfig{
		TenantDomains: cfg.TenantDomains,
	})

	// Instantiate the cryptographic signature validator middleware
	signatureValidator := middleware.NewSignatureValidator(sigVerifier, postgresStorage)

	// Wrap federated multi-tenant endpoints inside a protected routing group
	r.Group(func(protected chi.Router) {
		protected.Use(tenantValidator.Handler)

		// WebFinger Discovery Endpoint
		protected.Get("/.well-known/webfinger", inhttp.HandleWebfinger(cfg.TenantDomains))

		// Scoped activity routers
		protected.Route("/actors", func(router chi.Router) {
			router.Handle("/", actorHandler)
			router.Handle("/{actor}", actorHandler)
		})

		// Scoped cryptographic validation group explicitly targeting the /inbox delivery channel
		protected.Route("/inbox", func(router chi.Router) {
			router.Use(signatureValidator.Handler) // Enforces valid HTTP Signatures before task ingestion
			router.Handle("/", inboxHandler)
			router.Handle("/{actor}", inboxHandler)
		})
	})

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r, // Pass the Chi router tree directly into the server
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// 7. Graceful Shutdown Signal Handler
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Sprezz server running via Chi on http://localhost:%s for domains: %v", cfg.Port, cfg.TenantDomains)
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

	cancel() // Cancel context to safely stop background engines
	log.Println("Sprezz server stopped gracefully.")
}
