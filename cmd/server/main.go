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
	"sprezz/internal/adapters/in/worker"
	"sprezz/internal/adapters/out/cache"
	"sprezz/internal/adapters/out/federation"
	outhttp "sprezz/internal/adapters/out/http"
	"sprezz/internal/adapters/out/jsonld"
	"sprezz/internal/adapters/out/minio"
	"sprezz/internal/adapters/out/postgres"
	"sprezz/internal/config"
	"sprezz/internal/domain/service"
	"sprezz/internal/pkg/workers"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dependencies struct {
	cfg             *config.Config
	postgresStorage *postgres.PostgresStorage
	activityService *service.ActivityService
	federatedSigner *outhttp.FederatedSignerAdapter
}

func main() {
	log.Println("Starting Sprezz server...")

	// 1. Initialize configuration and structural dependencies
	deps, pool := initDependencies()
	defer pool.Close()

	// 2. Provision multi-tenant system boundaries and server actors [source: 2]
	log.Println("Validating configuration tenant boundaries and server identities...")
	bootstrapService := service.NewBootstrapService(deps.postgresStorage)
	if err := bootstrapService.BootstrapTenantsAndServerActors(context.Background(), deps.cfg.TenantDomains); err != nil {
		log.Fatalf("Critical failure bootstrapping tenant server actor layers: %v", err)
	}
	log.Println("Multi-tenant server actor matrices are completely provisioned.")

	// 3. Launch type-safe background batch processor loops [source: 3]
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startBackgroundWorkers(ctx, deps)

	// 4. Set up driving router and application server listening sockets [source: 3]
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.Timeout(60 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	setupRoutingTree(r, deps)

	server := &http.Server{
		Addr:         ":" + deps.cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// 5. Handle graceful termination signals
	handleShutdown(server, cancel)
}

func initDependencies() (*dependencies, *pgxpool.Pool) {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Configuration bootstrap error: %v", err)
	}

	dictCache, err := cache.NewDictionaryCache()
	if err != nil {
		log.Fatalf("Failed to initialize dictionary cache: %v", err)
	}

	dbConfig, err := pgxpool.ParseConfig(cfg.GetDSN())
	if err != nil {
		log.Fatalf("Failed to parse postgres configuration: %v", err)
	}
	dbConfig.MaxConns = 25
	dbConfig.MinConns = 10
	dbConfig.MaxConnLifetime = 5 * time.Minute

	log.Println("Connecting to database...")
	db, err := pgxpool.NewWithConfig(context.Background(), dbConfig)
	if err != nil {
		log.Fatalf("Failed to connect to postgres: %v", err)
	}

	if err := db.Ping(context.Background()); err != nil {
		log.Fatalf("Failed to ping postgres: %v", err)
	}

	log.Println("Executing database schema migration hooks...")
	if err := postgres.RunDatabaseMigrations(context.Background(), db); err != nil {
		log.Fatalf("Critical database schema migration failure: %v", err)
	}
	log.Println("Database schemas are synchronized and verified.")

	mediaStorage, err := minio.NewMinIOStorageAdapter(
		cfg.MinIO.Endpoint,
		cfg.MinIO.RootUser,
		cfg.MinIO.RootPassword,
		cfg.MinIO.BucketName,
		cfg.MinIO.UseSSL,
	)
	if err != nil {
		log.Fatalf("Critical storage adapter initialization error: %v", err)
	}

	postgresStorage := postgres.NewPostgresStorage(db, dictCache)
	jsonldParser := jsonld.NewJSONLDParser()
	federatedSigner := outhttp.NewFederatedSignerAdapter()
	activityService := service.NewActivityService(postgresStorage, jsonldParser, mediaStorage, federatedSigner)

	deps := &dependencies{
		cfg:             cfg,
		postgresStorage: postgresStorage,
		activityService: activityService,
		federatedSigner: federatedSigner,
	}

	return deps, db
}

func startBackgroundWorkers(ctx context.Context, deps *dependencies) {
	// Initialize the Inbound Driving Adapter using non-stuttering config
	inboundEngine := worker.NewInboundWorkerEngine(workers.Config{
		NumWorkers: 4,
		BatchSize:  10,
		PollDelay:  500 * time.Millisecond,
	}, deps.postgresStorage, deps.activityService)

	go func() {
		log.Println("Launching async Driving Inbound Worker Engine...")
		if err := inboundEngine.Start(ctx); err != nil {
			log.Printf("Inbound worker engine exited with error: %v", err)
		}
	}()

	// Initialize the Outbound Federation Driven Adapter using non-stuttering config
	outboundEngine := federation.NewOutboundWorkerEngine(workers.Config{
		NumWorkers: 4,
		BatchSize:  10,
		PollDelay:  500 * time.Millisecond,
	}, deps.postgresStorage, deps.federatedSigner)

	go func() {
		log.Println("Launching async Driven Outbound Federation Worker Engine...")
		if err := outboundEngine.Start(ctx); err != nil {
			log.Printf("Outbound worker engine exited with error: %v", err)
		}
	}()
}

func setupRoutingTree(r *chi.Mux, deps *dependencies) {
	federatedVerifier := inhttp.NewFederatedSignatureVerifier(deps.postgresStorage)
	genericHandler := inhttp.NewGenericHandler(deps.postgresStorage)

	tenantValidator := middleware.NewTenantValidator(middleware.TenantConfig{
		TenantDomains: deps.cfg.TenantDomains,
	})

	signatureValidator := middleware.NewSignatureValidator(federatedVerifier, deps.postgresStorage)

	r.Group(func(protected chi.Router) {
		protected.Use(tenantValidator.Handler)

		protected.Get("/.well-known/webfinger", inhttp.HandleWebfinger(deps.cfg.TenantDomains, deps.postgresStorage))

		// Unified Greedy Catch-All Endpoint (Handles GET & POST dynamically)
		protected.Route("/*", func(router chi.Router) {
			router.Use(signatureValidator.Handler)
			router.Get("/", genericHandler.ServeHTTP)
			router.Post("/", genericHandler.ServeHTTP)
		})
	})
}

func handleShutdown(server *http.Server, cancel context.CancelFunc) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Sprezz server running seamlessly via Chi on port 8080")
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

	cancel()
	log.Println("Sprezz server stopped gracefully.")
}
