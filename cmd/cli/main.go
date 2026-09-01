// Package cli provides the CLI entry point using Cobra.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/andrea20024/goferminutes2/internal/cli"
	"github.com/andrea20024/goferminutes2/internal/config"
	"github.com/andrea20024/goferminutes2/internal/logger"
	"github.com/andrea20024/goferminutes2/internal/mongo"
	"github.com/andrea20024/goferminutes2/internal/service"
	"github.com/andrea20024/goferminutes2/internal/storage"
	"github.com/spf13/cobra"
	mongov2 "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap/zapcore"
)

// Version info - set by linker.
var (
	version   = "dev"
	buildDate = "unknown"
)

func main() {
	// Initialize logger
	if err := logger.InitLogger(zapcore.InfoLevel); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	sugar := logger.Sugar()
	sugar.Infow("starting goferminutes2", "version", version, "buildDate", buildDate)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		sugar.Fatalf("Failed to load config: %v", err)
	}

	// Set the handlers factory before creating commands
	cli.SetHandlersFactory(func(cfg *config.Config) (*service.Handlers, error) {
		return initApp(cfg)
	})

	// Create root command
	rootCmd := &cobra.Command{
		Use:   "goferminutes2",
		Short: "Smart assistant for meeting note-taking",
		Long:  "CLI utility for loading, processing, and searching meeting materials.",
	}
	rootCmd.PersistentFlags().StringP("user-id", "u", "1", "User ID for CLI mode")

	// Register commands (no DB connection yet - lazy init)
	cli.RegisterCommands(rootCmd, cfg)

	// Channel for graceful shutdown signal
	shutdownCh := make(chan struct{})

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		sugar.Infow("signal received", "signal", sig)
		if globalService != nil {
			globalService.Stop()
		}
		if globalMongoClient != nil {
			_ = globalMongoClient.Disconnect(context.Background())
		}
		if globalDB != nil {
			_ = globalDB.Close()
		}
		close(shutdownCh)
	}()

	// Execute command
	if err := rootCmd.Execute(); err != nil {
		sugar.Errorf("Failed to execute command: %v", err)
		os.Exit(1)
	}

	// Graceful shutdown: stop service, disconnect MongoDB, close DB
	if globalService != nil {
		globalService.Stop()
	}
	if globalMongoClient != nil {
		_ = globalMongoClient.Disconnect(context.Background())
	}
	if globalDB != nil {
		_ = globalDB.Close()
	}
}

var (
	globalDB          *sql.DB
	globalRepo        *storage.Repository
	globalService     *service.MeetingService
	globalMongoClient *mongov2.Client
)

func initApp(cfg *config.Config) (*service.Handlers, error) {
	if globalService != nil {
		return service.GetHandlers(), nil
	}

	ctx := context.Background()

	// Connect to database
	db, err := storage.NewDB(ctx, cfg.DatabaseDSN)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	// Run migrations
	if err := storage.MigrateUpFromPath(cfg.MigrationsPath(), cfg.DatabaseDSN); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	logger.Sugar().Infow("migrations applied", "path", cfg.MigrationsPath())

	// Create repositories
	repo := storage.NewRepository(db)
	globalDB = db
	globalRepo = repo
	userRepo := storage.NewUserRepo(repo)
	meetingRepo := storage.NewMeetingRepo(repo)

	sugar := logger.Sugar()
	if sugar != nil {
		sugar.Infow("connected to database")
	}

	// Create external clients
	speechClient := service.CreateSpeechClient(cfg)
	llmClient := service.CreateLLMClient(cfg)

	// Connect to MongoDB for GridFS (optional)
	var gridFSClient *mongo.GridFSClient
	mongoCtx, mongoCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer mongoCancel()

	mongoClient, err := mongov2.Connect(options.Client().
		ApplyURI(cfg.MongoDBDSN))
	if err != nil {
		logger.Sugar().Warnw("failed to connect to MongoDB, GridFS disabled", "error", err)
	} else {
		if err := mongoClient.Ping(mongoCtx, nil); err != nil {
			logger.Sugar().Warnw("MongoDB ping failed, GridFS disabled", "error", err)
			_ = mongoClient.Disconnect(context.Background())
		} else {
			globalMongoClient = mongoClient
			mongoDB := mongoClient.Database(cfg.MongoDBDatabase)
			gridFSClient = mongo.NewGridFSClient(mongoDB, cfg.MongoDBBucket)
			logger.Sugar().Infow("connected to MongoDB GridFS",
				"database", cfg.MongoDBDatabase,
				"bucket", cfg.MongoDBBucket)
		}
	}

	// Create service
	meetingService := service.NewMeetingService(meetingRepo, userRepo, speechClient, llmClient, gridFSClient)
	globalService = meetingService

	// Create and store handlers
	h := &service.Handlers{
		Service:    meetingService,
		UserRepo:   userRepo,
		Repository: globalRepo,
		GridFS:     gridFSClient,
	}
	service.SetHandlers(h)

	return h, nil
}
