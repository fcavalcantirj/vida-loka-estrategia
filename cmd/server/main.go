package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3" // SQLite3 driver
	"github.com/user/vida-loka-strategy/config"
	"github.com/user/vida-loka-strategy/internal/game"
	"github.com/user/vida-loka-strategy/internal/whatsapp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "./config/config.json", "Path to configuration file")
	flag.Parse()

	// Set up logger
	logger := setupLogger()
	defer logger.Sync()

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	// Initialize game manager
	gameManager := game.NewGameManager(cfg)
	gameManager.SetLogger(logger) // Use SetLogger to properly initialize the event system

	// Load game data
	if err := loadGameData(gameManager, logger); err != nil {
		logger.Fatal("Failed to load game data", zap.Error(err))
	}

	// Initialize WhatsApp client manager
	clientManager := whatsapp.NewClientManager(gameManager, cfg, logger)

	// Connect GameManager with ClientManager using the MessageSender interface
	gameManager.SetMessageSender(clientManager)
	gameManager.SetClientManager(clientManager)

	// Initialize QR code manager
	qrManager := whatsapp.NewQRCodeManager(clientManager, cfg, logger)

	// Initialize session manager
	sessionManager := whatsapp.NewSessionManager(cfg.WhatsApp.StoreDir, logger)

	// Restore existing sessions
	if err := restoreExistingSessions(sessionManager, clientManager, logger); err != nil {
		logger.Error("Failed to restore existing sessions", zap.Error(err))
	}

	// Set up HTTP server for QR code generation and webhook handling
	server := setupHTTPServer(cfg, clientManager, qrManager, sessionManager, gameManager, logger)

	// Start HTTP server
	go func() {
		logger.Info("Starting HTTP server", zap.String("port", cfg.Server.Port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server stopped", zap.Error(err))
		}
	}()

	// Start the event system after everything else is initialized
	gameManager.StartEventSystem()
	defer gameManager.StopEventSystem()

	// Wait for shutdown signal
	waitForShutdown(logger)
}

func setupLogger() *zap.Logger {
	config := zap.NewProductionConfig()
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, _ := config.Build()
	return logger
}

func loadGameData(gameManager *game.GameManager, logger *zap.Logger) error {
	// Create data loader
	dataLoader := game.NewDataLoader("./assets/data")

	// Load characters
	characters, err := dataLoader.LoadCharacters()
	if err != nil {
		return fmt.Errorf("failed to load characters: %w", err)
	}
	gameManager.LoadCharacters(characters)
	logger.Info("Loaded characters", zap.Int("count", len(characters)))

	// Load events
	events, err := dataLoader.LoadEvents()
	if err != nil {
		return fmt.Errorf("failed to load events: %w", err)
	}
	gameManager.LoadEvents(events)
	logger.Info("Loaded events", zap.Int("count", len(events)))

	// Load actions
	actions, err := dataLoader.LoadActions()
	if err != nil {
		return fmt.Errorf("failed to load actions: %w", err)
	}
	gameManager.LoadActions(actions)
	logger.Info("Loaded actions", zap.Int("count", len(actions)))

	// Load zones
	zones, err := dataLoader.LoadZones()
	if err != nil {
		return fmt.Errorf("failed to load zones: %w", err)
	}
	gameManager.LoadZones(zones)
	logger.Info("Loaded zones", zap.Int("count", len(zones)))

	return nil
}

func restoreExistingSessions(sessionManager *whatsapp.SessionManager, clientManager *whatsapp.ClientManager, logger *zap.Logger) error {
	// List existing sessions
	sessions, err := sessionManager.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	logger.Info("Found existing sessions", zap.Int("count", len(sessions)))

	// Restore each session
	for _, session := range sessions {
		logger.Info("Restoring session",
			zap.String("phone_number", session.PhoneNumber),
			zap.String("session_id", session.ID))

		// Set up client
		client, err := clientManager.SetupClient(session.ID, session.PhoneNumber)
		if err != nil {
			logger.Error("Failed to set up client",
				zap.String("phone_number", session.PhoneNumber),
				zap.Error(err))
			continue
		}

		// Connect to WhatsApp
		if err := client.Connect(); err != nil {
			logger.Error("Failed to connect client",
				zap.String("phone_number", session.PhoneNumber),
				zap.Error(err))
			continue
		}

		// Check if logged in
		if !client.IsLoggedIn() {
			logger.Warn("Client not logged in",
				zap.String("phone_number", session.PhoneNumber))
			continue
		}

		logger.Info("Session restored successfully",
			zap.String("phone_number", session.PhoneNumber))
	}

	return nil
}

func setupHTTPServer(cfg config.Config, clientManager *whatsapp.ClientManager, qrManager *whatsapp.QRCodeManager, sessionManager *whatsapp.SessionManager, gameManager *game.GameManager, logger *zap.Logger) *http.Server {
	// Create router
	router := chi.NewRouter()
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(60 * time.Second))

	// Set up routes
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("Health check request received",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", r.RemoteAddr))
		w.Write([]byte("OK"))
	})

	router.Get("/healthcheck", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("Health check request received",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", r.RemoteAddr))
		w.Write([]byte("OK"))
	})

	// Serve static files
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./assets/templates/qr.html")
	})

	// Serve QR code images
	router.Get("/qrcodes/*", func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/qrcodes/", http.FileServer(http.Dir("./assets/qrcodes"))).ServeHTTP(w, r)
	})

	// QR code generation endpoint
	router.Post("/qr", func(w http.ResponseWriter, r *http.Request) {
		// Parse request
		var req struct {
			PhoneNumber string `json:"phone_number"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Generate session ID
		sessionID := uuid.New().String()

		// Generate QR code
		qrCode, err := qrManager.GenerateQRCode(sessionID, req.PhoneNumber)
		if err != nil {
			logger.Error("Failed to generate QR code",
				zap.String("phone_number", req.PhoneNumber),
				zap.Error(err))
			http.Error(w, "Failed to generate QR code", http.StatusInternalServerError)
			return
		}

		// Return QR code data
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"qr_code": qrCode,
		})
	})

	// Session management endpoints
	router.Get("/sessions", func(w http.ResponseWriter, r *http.Request) {
		sessions, err := sessionManager.ListSessions()
		if err != nil {
			logger.Error("Failed to list sessions", zap.Error(err))
			http.Error(w, "Failed to list sessions", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
	})

	router.Delete("/sessions/{phone_number}/{session_id}", func(w http.ResponseWriter, r *http.Request) {
		phoneNumber := chi.URLParam(r, "phone_number")
		sessionID := chi.URLParam(r, "session_id")

		// Disconnect client if connected
		if client, exists := clientManager.GetClient(phoneNumber); exists {
			client.Disconnect()
		}

		// Delete session
		if err := sessionManager.DeleteSession(phoneNumber, sessionID); err != nil {
			logger.Error("Failed to delete session",
				zap.String("phone_number", phoneNumber),
				zap.String("session_id", sessionID),
				zap.Error(err))
			http.Error(w, "Failed to delete session", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	// Create HTTP server
	return &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: router,
	}
}

func waitForShutdown(logger *zap.Logger) {
	// Set up channel for shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-sigChan
	logger.Info("Received shutdown signal", zap.String("signal", sig.String()))

	// Perform cleanup
	logger.Info("Shutting down")
}
