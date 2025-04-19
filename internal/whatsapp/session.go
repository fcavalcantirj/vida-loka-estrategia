package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/skip2/go-qrcode"
	"github.com/user/vida-loka-strategy/config"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	"go.uber.org/zap"
)

// QRCodeManager handles QR code generation and authentication
type QRCodeManager struct {
	clientManager *ClientManager
	config        config.Config
	logger        *zap.Logger
}

// NewQRCodeManager creates a new QR code manager
func NewQRCodeManager(clientManager *ClientManager, cfg config.Config, logger *zap.Logger) *QRCodeManager {
	return &QRCodeManager{
		clientManager: clientManager,
		config:        cfg,
		logger:        logger,
	}
}

// GenerateQRCode creates a QR code for WhatsApp authentication
func (qm *QRCodeManager) GenerateQRCode(sessionID, phoneNumber string) (string, error) {
	// Set up client if it doesn't exist
	client, exists := qm.clientManager.GetClient(phoneNumber)
	if !exists {
		var err error
		client, err = qm.clientManager.SetupClient(sessionID, phoneNumber)
		if err != nil {
			return "", fmt.Errorf("failed to set up client: %w", err)
		}
	}

	// Check if already logged in
	if client.IsLoggedIn() {
		return "", fmt.Errorf("client already logged in")
	}

	// Set up QR channel
	qrChan, err := client.GetQRChannel(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to get QR channel: %w", err)
	}

	// Connect to WhatsApp
	err = client.Connect()
	if err != nil {
		return "", fmt.Errorf("failed to connect: %w", err)
	}

	// Create QR code directory
	qrDir := filepath.Join(qm.config.WhatsApp.StoreDir, "qrcodes")
	if err := os.MkdirAll(qrDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create QR code directory: %w", err)
	}

	// Wait for QR code
	select {
	case evt := <-qrChan:
		if evt.Event == "code" {
			// Generate QR code image
			qrPath := filepath.Join(qrDir, fmt.Sprintf("%s_%s.png", phoneNumber, sessionID))
			err := qrcode.WriteFile(evt.Code, qrcode.Medium, 256, qrPath)
			if err != nil {
				return "", fmt.Errorf("failed to generate QR code image: %w", err)
			}

			qm.logger.Info("QR code generated",
				zap.String("phone_number", phoneNumber),
				zap.String("session_id", sessionID),
				zap.String("path", qrPath))

			return evt.Code, nil
		}
		return "", fmt.Errorf("unexpected QR event: %s", evt.Event)
	case <-time.After(60 * time.Second):
		return "", fmt.Errorf("timeout waiting for QR code")
	}
}

// SessionManager handles WhatsApp session management
type SessionManager struct {
	storeDir string
	logger   *zap.Logger
}

// NewSessionManager creates a new session manager
func NewSessionManager(storeDir string, logger *zap.Logger) *SessionManager {
	return &SessionManager{
		storeDir: storeDir,
		logger:   logger,
	}
}

// ListSessions returns all available WhatsApp sessions
func (sm *SessionManager) ListSessions() ([]SessionInfo, error) {
	// Create store directory if it doesn't exist
	if err := os.MkdirAll(sm.storeDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %w", err)
	}

	// List all database files
	pattern := filepath.Join(sm.storeDir, "store_*.db")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list session files: %w", err)
	}

	sessions := make([]SessionInfo, 0, len(matches))
	for _, match := range matches {
		// Extract session ID and phone number from filename
		filename := filepath.Base(match)
		var phoneNumber, sessionID string
		_, err := fmt.Sscanf(filename, "store_%s_%s.db", &phoneNumber, &sessionID)
		if err != nil {
			sm.logger.Warn("Failed to parse session filename", zap.String("filename", filename))
			continue
		}

		// Check if session is valid
		dbLog := waLog.Stdout("Database", "ERROR", true)
		container, err := sqlstore.New("sqlite3", "file:"+match+"?_foreign_keys=on", dbLog)
		if err != nil {
			sm.logger.Warn("Failed to open session database", zap.String("path", match))
			continue
		}

		deviceStore, err := container.GetFirstDevice()
		if err != nil {
			sm.logger.Warn("Failed to get device from session", zap.String("path", match))
			continue
		}

		// Add session to list
		sessions = append(sessions, SessionInfo{
			ID:          sessionID,
			PhoneNumber: phoneNumber,
			JID:         deviceStore.ID.String(),
			CreatedAt:   time.Now(), // This should ideally come from the database
		})
	}

	return sessions, nil
}

// SessionInfo holds information about a WhatsApp session
type SessionInfo struct {
	ID          string    `json:"id"`
	PhoneNumber string    `json:"phone_number"`
	JID         string    `json:"jid"`
	CreatedAt   time.Time `json:"created_at"`
}

// SaveSession persists session information
func (sm *SessionManager) SaveSession(session SessionInfo) error {
	// Create sessions directory
	sessionsDir := filepath.Join(sm.storeDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Marshal session to JSON
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Write to file
	path := filepath.Join(sessionsDir, fmt.Sprintf("%s_%s.json", session.PhoneNumber, session.ID))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// DeleteSession removes a WhatsApp session
func (sm *SessionManager) DeleteSession(phoneNumber, sessionID string) error {
	// Delete database file
	dbPath := filepath.Join(sm.storeDir, fmt.Sprintf("store_%s_%s.db", phoneNumber, sessionID))
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session database: %w", err)
	}

	// Delete session info file
	infoPath := filepath.Join(sm.storeDir, "sessions", fmt.Sprintf("%s_%s.json", phoneNumber, sessionID))
	if err := os.Remove(infoPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete session info: %w", err)
	}

	return nil
}

// MessageFormatter handles formatting game messages for WhatsApp
type MessageFormatter struct {
	// Add any configuration or dependencies needed for message formatting
}

// NewMessageFormatter creates a new message formatter
func NewMessageFormatter() *MessageFormatter {
	return &MessageFormatter{}
}

// FormatEventMessage formats an event message for WhatsApp
func (mf *MessageFormatter) FormatEventMessage(event *EventMessage) string {
	message := fmt.Sprintf("[%s]\n%s\n\n", event.Time.Format("15:04"), event.Description)

	if len(event.Options) > 0 {
		message += "Escolha sua ação:\n"
		for i, option := range event.Options {
			message += fmt.Sprintf("%s. %s\n", string('A'+i), option)
		}
	}

	return message
}

// FormatStatusMessage formats a status message for WhatsApp
func (mf *MessageFormatter) FormatStatusMessage(status *StatusMessage) string {
	message := fmt.Sprintf("📊 STATUS DE %s 📊\n\n", status.PlayerName)
	message += fmt.Sprintf("Personagem: %s (%s)\n", status.CharacterName, status.CharacterType)
	message += fmt.Sprintf("XP: %d\n", status.XP)
	message += fmt.Sprintf("Dinheiro: R$ %.2f\n", status.Money)
	message += fmt.Sprintf("Influência: %d\n", status.Influence)
	message += fmt.Sprintf("Estresse: %d/100\n", status.Stress)
	message += fmt.Sprintf("Localização: %s\n\n", status.Location)

	message += "ATRIBUTOS:\n"
	message += fmt.Sprintf("Carisma: %d\n", status.Attributes.Carisma)
	message += fmt.Sprintf("Proficiência: %d\n", status.Attributes.Proficiencia)
	message += fmt.Sprintf("Rede: %d\n", status.Attributes.Rede)
	message += fmt.Sprintf("Moralidade: %d\n", status.Attributes.Moralidade)
	message += fmt.Sprintf("Resiliência: %d\n", status.Attributes.Resiliencia)

	return message
}

// EventMessage represents an event message to be sent to a player
type EventMessage struct {
	Time        time.Time
	Description string
	Options     []string
}

// StatusMessage represents a player status message
type StatusMessage struct {
	PlayerName    string
	CharacterName string
	CharacterType string
	XP            int
	Money         int
	Influence     int
	Stress        int
	Location      string
	Attributes    struct {
		Carisma      int
		Proficiencia int
		Rede         int
		Moralidade   int
		Resiliencia  int
	}
}
