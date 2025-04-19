package whatsapp

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/user/vida-loka-strategy/config"
	"github.com/user/vida-loka-strategy/internal/interfaces"
	"github.com/user/vida-loka-strategy/internal/types"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// GameManager defines the interface for game operations
type GameManager interface {
	RegisterPlayer(phoneNumber, name string) (*types.Player, error)
	GetPlayer(phoneNumber string) (*types.Player, error)
	SelectCharacter(phoneNumber, characterID string) error
	PerformAction(phoneNumber, actionID string) (*types.Outcome, error)
	GenerateEvent(phoneNumber string) (*types.Event, error)
	ProcessEventChoice(phoneNumber, eventID, optionID string) (*types.Outcome, error)
	GetPlayerStatus(phoneNumber string) (map[string]interface{}, error)
	MovePlayer(playerID, zoneID, subZoneID string) error
	GetAvailableCharacters() []*types.Character
	GetAvailableActions(phoneNumber string) ([]*types.Action, error)
	GetZone(zoneID string) (*types.Zone, error)
	GetAllPlayers() []*types.Player
	TriggerRandomEvent(playerID string) (*types.Event, error)
	SendMessage(playerID string, message string) error
}

// ClientManager handles WhatsApp client connections
type ClientManager struct {
	clients     map[string]*ClientInfo
	gameManager interfaces.GameManager
	config      config.Config
	logger      *zap.Logger
	mutex       sync.RWMutex
}

// ClientInfo holds information about a WhatsApp client connection
type ClientInfo struct {
	UUID        string
	PhoneNumber string
	Client      *whatsmeow.Client
	Store       *store.Device
}

// NewClientManager creates a new WhatsApp client manager
func NewClientManager(gameManager GameManager, cfg config.Config, logger *zap.Logger) *ClientManager {
	cm := &ClientManager{
		clients:     make(map[string]*ClientInfo),
		gameManager: gameManager,
		config:      cfg,
		logger:      logger,
	}

	// Restore existing sessions
	cm.restoreExistingSessions()

	return cm
}

// restoreExistingSessions attempts to restore all existing WhatsApp sessions
func (cm *ClientManager) restoreExistingSessions() {
	// Create store directory if it doesn't exist
	if err := os.MkdirAll(cm.config.WhatsApp.StoreDir, 0755); err != nil {
		cm.logger.Error("Failed to create store directory", zap.Error(err))
		return
	}

	// Look for all database files in the store directory
	pattern := filepath.Join(cm.config.WhatsApp.StoreDir, "store_*.db")
	files, err := filepath.Glob(pattern)
	if err != nil {
		cm.logger.Error("Failed to scan for existing sessions", zap.Error(err))
		return
	}

	// Map to store the most recent session file for each phone number
	latestSessions := make(map[string]struct {
		file      string
		sessionID string
		modTime   time.Time
	})

	// Find the most recent session file for each phone number
	for _, file := range files {
		base := filepath.Base(file)
		parts := strings.Split(strings.TrimSuffix(base, ".db"), "_")
		if len(parts) < 3 {
			continue
		}
		phoneNumber := parts[1]
		sessionID := parts[2]

		// Get file modification time
		fileInfo, err := os.Stat(file)
		if err != nil {
			cm.logger.Error("Failed to get file info",
				zap.String("file", file),
				zap.Error(err))
			continue
		}

		// Update if this is the most recent file for this phone number
		if current, exists := latestSessions[phoneNumber]; !exists || fileInfo.ModTime().After(current.modTime) {
			latestSessions[phoneNumber] = struct {
				file      string
				sessionID string
				modTime   time.Time
			}{
				file:      file,
				sessionID: sessionID,
				modTime:   fileInfo.ModTime(),
			}
		}
	}

	// Clean up old session files and restore the most recent ones
	for phoneNumber, latest := range latestSessions {
		// Remove old session files for this phone number
		for _, file := range files {
			if strings.Contains(file, "store_"+phoneNumber+"_") && file != latest.file {
				if err := os.Remove(file); err != nil {
					cm.logger.Error("Failed to remove old session file",
						zap.String("file", file),
						zap.Error(err))
				} else {
					cm.logger.Info("Removed old session file",
						zap.String("file", file))
				}
			}
		}

		// Initialize database and store
		dbPath := fmt.Sprintf("file:%s/%s?_foreign_keys=on", cm.config.WhatsApp.StoreDir, filepath.Base(latest.file))
		dbLog := waLog.Stdout("Database", "INFO", true)
		container, err := sqlstore.New("sqlite3", dbPath, dbLog)
		if err != nil {
			cm.logger.Error("Failed to initialize database",
				zap.String("phoneNumber", phoneNumber),
				zap.Error(err))
			continue
		}

		// Get device store
		deviceStore, err := container.GetFirstDevice()
		if err != nil {
			cm.logger.Info("No valid session found in database",
				zap.String("phoneNumber", phoneNumber))
			continue
		}

		// Create client
		clientLog := waLog.Stdout("Client", "INFO", true)
		client := whatsmeow.NewClient(deviceStore, clientLog)

		// Set up event handler
		client.AddEventHandler(cm.handleWhatsAppEvent)

		// Store client info
		cm.mutex.Lock()
		cm.clients[phoneNumber] = &ClientInfo{
			UUID:        latest.sessionID,
			PhoneNumber: phoneNumber,
			Client:      client,
			Store:       deviceStore,
		}
		cm.mutex.Unlock()

		// Connect if we have a valid session
		if client.Store.ID != nil {
			go func(phone string, cli *whatsmeow.Client) {
				if err := cli.Connect(); err != nil {
					cm.logger.Error("Failed to connect restored client",
						zap.String("phoneNumber", phone),
						zap.Error(err))
					return
				}
				cm.logger.Info("Successfully connected restored client",
					zap.String("phoneNumber", phone))
			}(phoneNumber, client)
		} else {
			cm.logger.Info("Session requires QR code login",
				zap.String("phoneNumber", phoneNumber))
		}
	}
}

// SetupClient initializes a new WhatsApp client
func (cm *ClientManager) SetupClient(sessionID, phoneNumber string) (*whatsmeow.Client, error) {
	// Create store directory if it doesn't exist
	if err := os.MkdirAll(cm.config.WhatsApp.StoreDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %w", err)
	}

	// Create database path
	dbPath := fmt.Sprintf("file:%s/store_%s_%s.db?_foreign_keys=on", cm.config.WhatsApp.StoreDir, phoneNumber, sessionID)

	// Initialize database
	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New("sqlite3", dbPath, dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Get device store
	deviceStore, err := container.GetFirstDevice()
	if err != nil {
		deviceStore = container.NewDevice()
	}

	// Set device properties
	store.DeviceProps.RequireFullSync = proto.Bool(true)
	store.DeviceProps.Os = proto.String(cm.config.WhatsApp.ClientName)

	// Create client
	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	// Set up event handler
	client.AddEventHandler(cm.handleWhatsAppEvent)

	// Store client info
	cm.mutex.Lock()
	cm.clients[phoneNumber] = &ClientInfo{
		UUID:        sessionID,
		PhoneNumber: phoneNumber,
		Client:      client,
		Store:       deviceStore,
	}
	cm.mutex.Unlock()

	return client, nil
}

// GetClient retrieves a WhatsApp client by phone number
func (cm *ClientManager) GetClient(phoneNumber string) (*whatsmeow.Client, bool) {
	cm.mutex.RLock()
	clientInfo, exists := cm.clients[phoneNumber]
	cm.mutex.RUnlock()

	if !exists {
		return nil, false
	}

	// If client exists but not connected, try to connect
	if !clientInfo.Client.IsConnected() && clientInfo.Store.ID != nil {
		if err := clientInfo.Client.Connect(); err != nil {
			cm.logger.Error("Failed to connect client",
				zap.String("phoneNumber", phoneNumber),
				zap.Error(err))
			return nil, false
		}
		cm.logger.Info("Successfully reconnected client",
			zap.String("phoneNumber", phoneNumber))
	}

	return clientInfo.Client, true
}

// GetQRChannel sets up a QR code channel for client authentication
func (cm *ClientManager) GetQRChannel(phoneNumber string) (<-chan whatsmeow.QRChannelItem, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// If client exists, disconnect and remove it
	if clientInfo, exists := cm.clients[phoneNumber]; exists {
		clientInfo.Client.Disconnect()
		delete(cm.clients, phoneNumber)
	}

	// Generate a new session ID
	sessionID := uuid.New().String()

	// Create database path
	dbPath := fmt.Sprintf("file:%s/store_%s_%s.db?_foreign_keys=on", cm.config.WhatsApp.StoreDir, phoneNumber, sessionID)

	// Initialize database
	dbLog := waLog.Stdout("Database", "INFO", true)
	container, err := sqlstore.New("sqlite3", dbPath, dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Create new device store
	deviceStore := container.NewDevice()

	// Set device properties
	store.DeviceProps.RequireFullSync = proto.Bool(true)
	store.DeviceProps.Os = proto.String(cm.config.WhatsApp.ClientName)

	// Create client
	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	// Set up event handler
	client.AddEventHandler(cm.handleWhatsAppEvent)

	// Get QR channel before storing or connecting
	qrChan, err := client.GetQRChannel(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get QR channel: %w", err)
	}

	// Store client info
	cm.clients[phoneNumber] = &ClientInfo{
		UUID:        sessionID,
		PhoneNumber: phoneNumber,
		Client:      client,
		Store:       deviceStore,
	}

	// Connect in a goroutine
	go func() {
		if err := client.Connect(); err != nil {
			cm.logger.Error("Failed to connect client",
				zap.String("phoneNumber", phoneNumber),
				zap.Error(err))
			return
		}

		cm.logger.Info("Client connected successfully",
			zap.String("phoneNumber", phoneNumber))
	}()

	return qrChan, nil
}

// Connect establishes a connection to WhatsApp
func (cm *ClientManager) Connect(phoneNumber string) error {
	client, exists := cm.GetClient(phoneNumber)
	if !exists {
		return fmt.Errorf("client not found for phone number: %s", phoneNumber)
	}

	return client.Connect()
}

// Disconnect closes a specific WhatsApp connection
func (cm *ClientManager) Disconnect(phoneNumber string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	clientInfo, exists := cm.clients[phoneNumber]
	if !exists {
		return fmt.Errorf("client not found for phone number: %s", phoneNumber)
	}

	clientInfo.Client.Disconnect()
	delete(cm.clients, phoneNumber)
	return nil
}

// DisconnectAll closes all WhatsApp connections
func (cm *ClientManager) DisconnectAll() {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for phoneNumber, clientInfo := range cm.clients {
		if clientInfo.Client != nil {
			clientInfo.Client.Disconnect()
			cm.logger.Info("Disconnected client", zap.String("phoneNumber", phoneNumber))
		}
	}

	cm.clients = make(map[string]*ClientInfo)
}

// IsLoggedIn checks if a client is logged in
func (cm *ClientManager) IsLoggedIn(phoneNumber string) (bool, error) {
	client, exists := cm.GetClient(phoneNumber)
	if !exists {
		return false, fmt.Errorf("client not found for phone number: %s", phoneNumber)
	}

	return client.IsLoggedIn(), nil
}

// SendTextMessage sends a text message to a WhatsApp user
func (cm *ClientManager) SendTextMessage(phoneNumber, recipient, message string) (string, error) {
	client, exists := cm.GetClient(phoneNumber)
	if !exists {
		return "", fmt.Errorf("client not found for phone number: %s", phoneNumber)
	}

	// Format recipient phone number
	// Remove any non-digit characters
	recipient = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, recipient)

	// Add country code if not present
	if !strings.HasPrefix(recipient, "55") {
		recipient = "55" + recipient
	}

	// Parse recipient JID
	recipientJID, err := parseJID(recipient)
	if err != nil {
		return "", fmt.Errorf("failed to parse recipient JID: %w", err)
	}

	// Create message
	msg := &waProto.Message{
		Conversation: proto.String(message),
	}

	// Send message
	response, err := client.SendMessage(context.Background(), recipientJID, msg)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	return response.ID, nil
}

// handleWhatsAppEvent processes incoming WhatsApp events
func (cm *ClientManager) handleWhatsAppEvent(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		cm.handleIncomingMessage(v)
	case *events.Connected:
		cm.logger.Info("WhatsApp client connected")
	case *events.Disconnected:
		cm.logger.Info("WhatsApp client disconnected")
	case *events.LoggedOut:
		cm.logger.Info("WhatsApp client logged out")
	}
}

// handleIncomingMessage processes incoming WhatsApp messages
func (cm *ClientManager) handleIncomingMessage(message *events.Message) {
	// Skip messages sent by this bot
	if message.Info.MessageSource.IsFromMe {
		return
	}

	// Get message content
	content := message.Message.GetConversation()
	if content == "" && message.Message.ExtendedTextMessage != nil {
		content = *message.Message.ExtendedTextMessage.Text
	}

	// Skip empty messages
	if content == "" {
		return
	}

	// Check if this is a group message
	isGroup := message.Info.Chat.Server == "g.us"

	// For group messages, only process if it starts with '/ '
	if isGroup {
		if !strings.HasPrefix(content, "/ ") {
			return
		}
		// Remove the '/ ' prefix for group messages
		content = strings.TrimPrefix(content, "/ ")
	} else {
		// For private messages, check for '/' prefix
		if !strings.HasPrefix(content, "/") {
			return
		}
	}

	// Log message details
	cm.logger.Debug("Received message",
		zap.String("content", content),
		zap.String("sender", message.Info.Sender.User),
		zap.String("chat", message.Info.Chat.User))

	// Process command
	response := cm.processGameCommand(message.Info.Sender.User, content)
	if response != "" {
		// Get the first client from the manager (our bot's client)
		cm.mutex.RLock()
		var client *whatsmeow.Client
		for _, clientInfo := range cm.clients {
			if clientInfo.Client.IsLoggedIn() && clientInfo.Client.Store.ID != nil {
				client = clientInfo.Client
				break
			}
		}
		cm.mutex.RUnlock()

		// If no client is available, log the error but don't try to send a QR code
		if client == nil {
			cm.logger.Error("No valid client available to send response",
				zap.String("sender", message.Info.Sender.User),
				zap.String("response", response))
			return
		}

		// Send response
		targetJID := message.Info.Chat
		msg := &waProto.Message{
			Conversation: proto.String(response),
		}

		_, err := client.SendMessage(context.Background(), targetJID, msg)
		if err != nil {
			cm.logger.Error("Failed to send response",
				zap.String("sender", message.Info.Sender.User),
				zap.Error(err))
		}
	}
}

// processGameCommand handles game commands from players
func (cm *ClientManager) processGameCommand(sender, command string) string {
	// Add logging for all incoming commands
	cm.logger.Info("Processing game command",
		zap.String("sender", sender),
		zap.String("command", command))

	// Clean and normalize command
	command = cleanCommand(command)

	// Check if command starts with '/'
	if !strings.HasPrefix(command, "/") {
		cm.logger.Debug("Command doesn't start with /",
			zap.String("command", command))
		return "Comandos devem começar com '/'. Digite '/ajuda' para ver os comandos disponíveis."
	}

	// Remove the '/' prefix
	command = strings.TrimPrefix(command, "/")

	// Log the cleaned command
	cm.logger.Debug("Processing cleaned command",
		zap.String("cleaned_command", command))

	// Check if this is a setup command (hidden from help)
	if command == "setup" {
		cm.logger.Info("Handling setup command",
			zap.String("sender", sender))
		return cm.handleSetupCommand(sender, command)
	}

	// Check if this is a help command
	if command == "ajuda" || command == "help" {
		cm.logger.Info("Handling help command")
		return cm.handleHelpCommand()
	}

	// Check if this is a move command
	if strings.HasPrefix(command, "mover") {
		cm.logger.Info("Handling move command",
			zap.String("command", command))
		return cm.handleMoveCommand(sender, command)
	}

	// Check if this is a registration command
	if strings.HasPrefix(command, "comecar") || strings.HasPrefix(command, "começar") || strings.HasPrefix(command, "iniciar") {
		cm.logger.Info("Handling registration command",
			zap.String("command", command))
		return cm.handleRegistrationCommand(sender, command)
	}

	// Check if this is a character selection command
	if strings.HasPrefix(command, "escolher") {
		cm.logger.Info("Handling character selection command",
			zap.String("command", command))
		return cm.handleCharacterSelectionCommand(sender, command)
	}

	// Check if this is a status command
	if command == "status" {
		cm.logger.Info("Handling status command")
		return cm.handleStatusCommand(sender)
	}

	// Check if this is a characters list command
	if command == "personagens" {
		cm.logger.Info("Handling characters list command")
		return cm.handleCharactersListCommand()
	}

	// Check if this is an action command
	if strings.HasPrefix(command, "trabalhar") ||
		strings.HasPrefix(command, "estudar") ||
		strings.HasPrefix(command, "relaxar") ||
		strings.HasPrefix(command, "curtir") ||
		strings.HasPrefix(command, "dormir") ||
		strings.HasPrefix(command, "meditar") ||
		strings.HasPrefix(command, "networking") ||
		strings.HasPrefix(command, "treinar") ||
		strings.HasPrefix(command, "empreender") ||
		strings.HasPrefix(command, "ajudar") {
		cm.logger.Info("Handling action command",
			zap.String("command", command))
		return cm.handleActionCommand(sender, command)
	}

	// Check if this is an event response
	if command == "a" || command == "b" || command == "c" || command == "d" {
		cm.logger.Info("Handling event response command",
			zap.String("command", command))
		return cm.handleEventResponseCommand(sender, command)
	}

	// Unknown command
	cm.logger.Info("Unknown command received",
		zap.String("command", command))
	return "Comando não reconhecido. Digite '/ajuda' para ver os comandos disponíveis."
}

// handleRegistrationCommand processes player registration
func (cm *ClientManager) handleRegistrationCommand(sender, command string) string {
	parts := strings.SplitN(command, " ", 2)
	if len(parts) < 2 {
		return "Ei, você esqueceu seu nome! 🧐\n\n" +
			"Digite: */comecar [seu nome]*"
	}

	playerName := parts[1]

	// Check if player is already registered
	existingPlayer, _ := cm.gameManager.GetPlayer(sender)
	if existingPlayer != nil {
		return "Calma aí, você já está no jogo! 😅\n\n" +
			"Digite */status* para ver sua situação atual."
	}

	// Register new player
	player, err := cm.gameManager.RegisterPlayer(sender, playerName)
	if err != nil {
		return fmt.Sprintf("Ops! Algo deu errado: %s 😱", err.Error())
	}

	// Get available characters
	characters := cm.gameManager.GetAvailableCharacters()
	var characterList strings.Builder
	characterList.WriteString("🎭 *PERSONAGENS DISPONÍVEIS* 🎭\n\n")
	for i, char := range characters {
		characterList.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, getCharacterEmoji(char.Name), char.Name))
		characterList.WriteString(fmt.Sprintf("   %s\n\n", char.Description))
	}
	characterList.WriteString("Digite */escolher [número]* para escolher seu personagem.")

	// Send welcome message without QR code
	return fmt.Sprintf("E aí, %s! Bem-vindo ao *VIDA LOKA STRATEGY*! 🎮\n\n"+
		"Agora você pode escolher seu personagem.\n\n%s", player.Name, characterList.String())
}

// handleSetupCommand sets up the host's WhatsApp connection
func (cm *ClientManager) handleSetupCommand(sender, command string) string {
	// Get QR channel for WhatsApp authentication
	qrChan, err := cm.GetQRChannel(sender)
	if err != nil {
		return fmt.Sprintf("❌ *Erro na Configuração* ❌\n\n"+
			"Não consegui configurar o WhatsApp: %s", err.Error())
	}

	// Start QR code login process in a goroutine
	go func() {
		for evt := range qrChan {
			if evt.Event == "code" {
				// Format QR code message
				message := fmt.Sprintf("📱 *CONFIGURAÇÃO DO HOST* 📱\n\n"+
					"Para configurar o bot do jogo:\n\n"+
					"1. Abra o WhatsApp no seu celular\n"+
					"2. Vá em *Menu* > *WhatsApp Web*\n"+
					"3. Escaneie este código QR:\n\n"+
					"```\n%s\n```\n\n"+
					"Depois de escanear, o bot estará pronto para enviar mensagens! 🤖", evt.Code)

				// Send QR code to host
				if err := cm.gameManager.SendMessage(sender, message); err != nil {
					cm.logger.Error("Failed to send QR code to host",
						zap.String("phone_number", sender),
						zap.Error(err))
				}
			} else if evt.Event == "success" {
				// Store the host number in config
				cm.config.WhatsApp.HostNumber = sender
				if err := config.SaveConfig(cm.config, "config.json"); err != nil {
					cm.logger.Error("Failed to save host number to config",
						zap.String("phone_number", sender),
						zap.Error(err))
				}

				cm.logger.Info("Host WhatsApp client successfully authenticated",
					zap.String("phone_number", sender))
				// Connect the client after successful authentication
				if err := cm.Connect(sender); err != nil {
					cm.logger.Error("Failed to connect host WhatsApp client",
						zap.String("phone_number", sender),
						zap.Error(err))
				}
			}
		}
	}()

	return "🔄 *Iniciando Configuração* 🔄\n\n" +
		"Preparando o bot para enviar mensagens...\n" +
		"Você receberá um código QR em breve."
}

// handleCharacterSelectionCommand processes character selection
func (cm *ClientManager) handleCharacterSelectionCommand(sender, command string) string {
	// Extract character number from command
	parts := strings.SplitN(command, " ", 2)
	if len(parts) < 2 {
		return "Para escolher um personagem, digite: escolher [número]"
	}

	// Get available characters
	characters := cm.gameManager.GetAvailableCharacters()

	// Parse character number
	var characterIndex int
	_, err := fmt.Sscanf(parts[1], "%d", &characterIndex)
	if err != nil || characterIndex < 1 || characterIndex > len(characters) {
		return fmt.Sprintf("Número de personagem inválido. Escolha entre 1 e %d.", len(characters))
	}

	// Select character (adjust index to 0-based)
	selectedCharacter := characters[characterIndex-1]
	err = cm.gameManager.SelectCharacter(sender, selectedCharacter.ID)
	if err != nil {
		if err.Error() == "player already has character selected" {
			return "Você já escolheu um personagem. Digite 'status' para ver sua situação atual."
		}
		return fmt.Sprintf("Erro ao selecionar personagem: %s", err.Error())
	}

	// Send character selection confirmation
	response := fmt.Sprintf("🎉 *PARABÉNS!* Você agora é *%s* %s\n\n"+
		"*Seus atributos:*\n"+
		"Carisma: %d 🎭\n"+
		"Proficiência: %d 🧠\n"+
		"Rede: %d 🤝\n"+
		"Moralidade: %d 👼\n"+
		"Resiliência: %d 🥊\n\n"+
		"Você acorda em *Copacabana* 🌊 com R$ 100,00 💰 e 0 XP ⭐\n\n"+
		"Digite */ajuda* para ver os comandos disponíveis! 🎮",
		selectedCharacter.Name, getCharacterEmoji(selectedCharacter.Name),
		selectedCharacter.Carisma,
		selectedCharacter.Proficiencia,
		selectedCharacter.Rede,
		selectedCharacter.Moralidade,
		selectedCharacter.Resiliencia)

	return response
}

// handleStatusCommand processes status requests
func (cm *ClientManager) handleStatusCommand(sender string) string {
	// Get player status
	status, err := cm.gameManager.GetPlayerStatus(sender)
	if err != nil {
		if err.Error() == "player not found" {
			return "Ei, você nem começou o jogo ainda! 😅\nDigite 'comecar [seu nome]' para começar."
		}
		if err.Error() == "player has no character selected" {
			return "Você ainda não escolheu um personagem! 🤔\nUse 'escolher [número]' para selecionar."
		}
		return fmt.Sprintf("Ops! Algo deu errado: %s 😱", err.Error())
	}

	// Build response
	response := fmt.Sprintf("📊 STATUS DE %s 📊\n\n", status["name"])
	response += fmt.Sprintf("Personagem: %s (%s) 🎭\n", status["character"], status["character_type"])
	response += fmt.Sprintf("XP: %d ⭐\n", status["xp"])
	response += fmt.Sprintf("Dinheiro: R$ %d,00 💵\n", status["money"])
	response += fmt.Sprintf("Influência: %d 🎭\n", status["influence"])
	response += fmt.Sprintf("Estresse: %d/100 💥\n", status["stress"])
	response += fmt.Sprintf("Localização: %s 🗺️\n\n", status["location"])

	response += "ATRIBUTOS:\n"
	attributes := status["attributes"].(map[string]int)
	response += fmt.Sprintf("Carisma: %d 🎭\n", attributes["carisma"])
	response += fmt.Sprintf("Proficiência: %d 🧠\n", attributes["proficiencia"])
	response += fmt.Sprintf("Rede: %d 🤝\n", attributes["rede"])
	response += fmt.Sprintf("Moralidade: %d 👼\n", attributes["moralidade"])
	response += fmt.Sprintf("Resiliência: %d 🥊\n", attributes["resiliencia"])

	return response
}

// handleActionCommand processes player actions
func (cm *ClientManager) handleActionCommand(sender, command string) string {
	// Get player
	player, err := cm.gameManager.GetPlayer(sender)
	if err != nil {
		return "Ei, você nem começou o jogo ainda! 😅\n\n" +
			"Use */comecar [seu nome]* pra começar sua jornada!"
	}

	// Check if player has a character
	if player.CurrentCharacter == nil {
		return "Você ainda não escolheu um personagem! 🤔\n\n" +
			"Use */personagens* pra ver quem você pode ser!"
	}

	// Clean command
	command = cleanCommand(command)
	// Remove slash and convert to lowercase
	command = strings.TrimPrefix(strings.ToLower(command), "/")

	// Get available actions for current location
	availableActions, err := cm.gameManager.GetAvailableActions(sender)
	if err != nil {
		return fmt.Sprintf("Ops! Algo deu errado: %s 😱", err.Error())
	}

	// Check if the action is available in current zone
	isAvailable := false
	var actionNames []string
	for _, action := range availableActions {
		actionNames = append(actionNames, action.Name)
		if command == action.Name {
			isAvailable = true
			break
		}
	}

	if !isAvailable {
		// Format the subzone name properly
		displayName := strings.Title(strings.ReplaceAll(player.CurrentSubZone, "_", " "))
		actionList := strings.Join(actionNames, ", ")

		return fmt.Sprintf("❌ *Ação não disponível em %s!*\n\n"+
			"*Ações disponíveis aqui:*\n%s\n\n"+
			"Use */mover [subzona]* para ir para outro lugar! 🏃‍♂️",
			displayName, actionList)
	}

	// Perform action
	outcome, err := cm.gameManager.PerformAction(sender, command)
	if err != nil {
		return fmt.Sprintf("Ops! Não deu pra fazer isso: %s 😱", err.Error())
	}

	// Build response
	response := fmt.Sprintf("🎯 *%s*", outcome.Description)

	if outcome.XPChange != 0 {
		response += fmt.Sprintf("\n⭐ XP: %+d", outcome.XPChange)
	}

	if outcome.MoneyChange != 0 {
		response += fmt.Sprintf("\n💰 Dinheiro: R$ %+d,00", outcome.MoneyChange)
	}

	if outcome.InfluenceChange != 0 {
		response += fmt.Sprintf("\n🎭 Influência: %+d", outcome.InfluenceChange)
	}

	if outcome.StressChange != 0 {
		response += fmt.Sprintf("\n💥 Estresse: %+d", outcome.StressChange)
	}

	return response
}

// handleEventResponseCommand processes player responses to events
func (cm *ClientManager) handleEventResponseCommand(sender, command string) string {
	// Clean command
	command = cleanCommand(command)
	// Remove slash and convert to lowercase
	command = strings.TrimPrefix(strings.ToLower(command), "/")
	// Take only the first character
	if len(command) > 0 {
		command = string(command[0])
	}

	// Get player
	player, err := cm.gameManager.GetPlayer(sender)
	if err != nil {
		return fmt.Sprintf("Ops! Não consegui encontrar seu personagem: %v 😱", err)
	}

	// Check if player has a character
	if player.CurrentCharacter == nil {
		return "Você ainda não escolheu um personagem! 🤔\n\n" +
			"Use */personagens* pra ver quem você pode ser!"
	}

	// Get the current event from the player's state
	if player.CurrentEvent == nil {
		return "Você não tem nenhum evento pendente! 🎭\n\n" +
			"Continue explorando o mundo para encontrar eventos!"
	}

	// Map command letter to option index
	optionIndex := -1
	switch command {
	case "a":
		optionIndex = 0
	case "b":
		optionIndex = 1
	case "c":
		optionIndex = 2
	case "d":
		optionIndex = 3
	default:
		return "Opção inválida! Use /a, /b, /c ou /d para responder ao evento! 🎲"
	}

	// Check if option index is valid
	if optionIndex >= len(player.CurrentEvent.Options) {
		return "Essa opção não está disponível para este evento! 🎲"
	}

	// Get the option ID from the current event
	optionID := player.CurrentEvent.Options[optionIndex].ID

	// Store event ID before clearing it
	eventID := player.CurrentEvent.ID

	// Clear the current event before processing to prevent double-processing
	player.CurrentEvent = nil

	// Send dice rolling message
	diceMessage := "🎲 *ROLANDO OS DADOS...* 🎲\n\n" +
		"O destino está sendo decidido...\n" +
		"Os deuses do RNG estão trabalhando...\n" +
		"*TUM TUM TUM...*"

	// Get client to send dice message
	cm.mutex.RLock()
	var client *whatsmeow.Client
	for _, clientInfo := range cm.clients {
		if clientInfo.Client.IsLoggedIn() && clientInfo.Client.Store.ID != nil {
			client = clientInfo.Client
			break
		}
	}
	cm.mutex.RUnlock()

	if client != nil {
		// Send dice message
		msg := &waProto.Message{
			Conversation: proto.String(diceMessage),
		}
		client.SendMessage(context.Background(), waTypes.NewJID(sender, "s.whatsapp.net"), msg)

		// Add a small delay for dramatic effect
		time.Sleep(2 * time.Second)
	}

	// Process event choice
	outcome, err := cm.gameManager.ProcessEventChoice(sender, eventID, optionID)
	if err != nil {
		// If there's an error, we should NOT restore the event since it might be invalid
		return fmt.Sprintf("Ops! Algo deu errado: %v 😱", err)
	}

	// Build response
	response := fmt.Sprintf("🎭 *RESULTADO DO EVENTO* 🎭\n\n")
	response += fmt.Sprintf("%s\n\n", outcome.Description)

	if outcome.XPChange != 0 {
		response += fmt.Sprintf("XP: %+d ⭐\n", outcome.XPChange)
	}

	if outcome.MoneyChange != 0 {
		response += fmt.Sprintf("Dinheiro: %+d 💵\n", outcome.MoneyChange)
	}

	if outcome.InfluenceChange != 0 {
		response += fmt.Sprintf("Influência: %+d 🎭\n", outcome.InfluenceChange)
	}

	if outcome.StressChange != 0 {
		response += fmt.Sprintf("Estresse: %+d 💥\n", outcome.StressChange)
	}

	return response
}

// handleCharactersListCommand returns the list of available characters
func (cm *ClientManager) handleCharactersListCommand() string {
	characters := cm.gameManager.GetAvailableCharacters()

	response := "🎭 *PERSONAGENS DISPONÍVEIS* 🎭\n\n"
	response += "Para escolher um personagem, digite: */escolher [número]* 🎯\n\n"

	for i, char := range characters {
		emoji := getCharacterEmoji(char.Name)

		response += fmt.Sprintf("%d. *%s* %s\n", i+1, char.Name, emoji)
		response += fmt.Sprintf("   Carisma: %d 🎭\n", char.Carisma)
		response += fmt.Sprintf("   Proficiência: %d 🧠\n", char.Proficiencia)
		response += fmt.Sprintf("   Rede: %d 🤝\n", char.Rede)
		response += fmt.Sprintf("   Moralidade: %d 👼\n", char.Moralidade)
		response += fmt.Sprintf("   Resiliência: %d 🥊\n\n", char.Resiliencia)
	}

	response += "Para escolher um personagem, digite: */escolher [número]* 🎯"

	return response
}

// handleHelpCommand returns help information
func (cm *ClientManager) handleHelpCommand() string {
	response := "🎮 *VIDA LOKA STRATEGY* - SEU GUIA DE SOBREVIVÊNCIA 🎮\n\n"

	response += "🎯 *BÁSICOS* (PRA NÃO FICAR PERDIDO):\n"
	response += "*/comecar [nome]* - Começa sua jornada de sucesso (ou fracasso) 🚀\n"
	response += "*/começar [nome]* - Mesma coisa, só que com acento (pra quem é chique) 🎩\n"
	response += "*/personagens* - Conheça os malucos que você pode ser 🎭\n"
	response += "*/escolher [número]* - Escolha seu personagem (escolha sabiamente) 🤔\n"
	response += "*/status* - Veja como tá sua vida (ou o que sobrou dela) 📊\n"
	response += "*/ajuda* - Tá perdido? Chama o tio aqui! 🆘\n\n"

	response += "💪 *AÇÕES PRINCIPAIS* (PRA GANHAR DINHEIRO):\n"
	response += "*/trabalhar* - Trabalhe que nem um condenado 💼\n"
	response += "*/estudar* - Estude que nem um nerd (mas vale a pena) 📚\n"
	response += "*/relaxar* - Relaxe antes que você exploda 🧘‍♂️\n"
	response += "*/curtir* - Curta a vida (mas não muito) 🎉\n"
	response += "*/dormir* - Durma que nem um bebê (ou um morto) 😴\n\n"

	response += "✨ *AÇÕES ADICIONAIS* (PRA FICAR MAIS FODA):\n"
	response += "*/meditar* - Fique zen que nem um monge 🧘‍♂️\n"
	response += "*/networking* - Faça amizades (ou inimigos) 🤝\n"
	response += "*/treinar* - Fique forte que nem o Hulk 💪\n"
	response += "*/empreender* - Vire o próximo Elon Musk (ou não) 🚀\n"
	response += "*/ajudar* - Seja bonzinho (ou não) 👼\n\n"

	response += "🏃‍♂️ *ZONAS E LOCOMOÇÃO* (PRA NÃO FICAR PARADO):\n"
	response += "*/mover [subzona]* - Mude de lugar (antes que te peguem) 🏃‍♂️\n"
	response += "Zona Sul: Copacabana, Ipanema, Leblon, Vidigal 🌊\n"
	response += "Zona Norte: Madureira, Méier, Complexo do Alemão, Tijuca 🏙️\n"
	response += "Centro: Lapa, SAARA, Cinelândia, Porto Maravilha 🎭\n"
	response += "Zona Oeste: Barra da Tijuca, Jacarepaguá, Campo Grande, Santa Cruz 🌅\n\n"

	response += "🎲 *ATRIBUTOS* (PRA FICAR MAIS INTELIGENTE):\n"
	response += "Carisma: Habilidade de convencer até pedra 🎭\n"
	response += "Proficiência: Saber fazer as coisas (ou fingir que sabe) 🧠\n"
	response += "Rede: Ter amigos em todo lugar (ou inimigos) 🤝\n"
	response += "Moralidade: Ser bonzinho (ou não) 👼\n"
	response += "Resiliência: Aguentar pancada que nem um campeão 🥊\n\n"

	response += "💰 *RECURSOS* (PRA NÃO FICAR NA MISÉRIA):\n"
	response += "XP: Experiência de vida (ou de morte) ⭐\n"
	response += "Dinheiro: O que move o mundo (e seu jogo) 💵\n"
	response += "Influência: Poder de convencer os outros 🎭\n"
	response += "Estresse: O que te faz explodir 💥\n\n"

	response += "🎭 *EVENTOS* (PRA NÃO FICAR ENTEDIADO):\n"
	response += "Responda a eventos com */a*, */b*, */c* ou */d* 🎲\n"
	response += "Sucesso = 1d20 + atributo relevante (boa sorte!) 🍀\n\n"

	response += "Boa sorte na sua jornada! Que a força esteja com você! 🍀✨"
	return response
}

// handleMoveCommand processes player movement between zones
func (cm *ClientManager) handleMoveCommand(sender, command string) string {
	// Clean up the command by removing extra spaces
	command = strings.TrimSpace(command)
	parts := strings.Fields(command)
	if len(parts) < 2 {
		return "Ei, você esqueceu pra onde vai! 🧐\n\n" +
			"*Zonas disponíveis:*\n\n" +
			"• *Zona Sul:* Copacabana, Ipanema, Leblon, Vidigal 🌊\n" +
			"• *Zona Norte:* Madureira, Méier, Complexo do Alemão, Tijuca 🏙️\n" +
			"• *Centro:* Lapa, SAARA, Cinelândia, Porto Maravilha 🎭\n" +
			"• *Zona Oeste:* Barra da Tijuca, Jacarepaguá, Campo Grande, Santa Cruz 🌅\n\n" +
			"Use: */mover [subzona]*\n" +
			"Exemplo: */mover ipanema*"
	}

	// Join all parts after "mover" to handle multi-word subzones
	subZoneInput := strings.ToLower(strings.Join(parts[1:], " "))

	// Check if the user is trying to move to a zone instead of a subzone
	zoneNames := map[string]string{
		"zona sul":   "zona_sul",
		"zona norte": "zona_norte",
		"centro":     "centro",
		"zona oeste": "zona_oeste",
	}

	if zoneID, isZone := zoneNames[subZoneInput]; isZone {
		return fmt.Sprintf("Ei, você precisa escolher uma subzona específica! 🗺️\n\n"+
			"*Subzonas disponíveis em %s:*\n\n"+
			"%s\n\n"+
			"Use: */mover [subzona]*\n"+
			"Exemplo: */mover ipanema*",
			strings.Title(subZoneInput),
			getSubzonesForZone(zoneID))
	}

	// Normalize the subzone ID
	subZoneID := normalizeSubzoneName(subZoneInput)

	zoneMap := map[string]string{
		"copacabana":      "zona_sul",
		"ipanema":         "zona_sul",
		"leblon":          "zona_sul",
		"vidigal":         "zona_sul",
		"madureira":       "zona_norte",
		"meier":           "zona_norte",
		"complexo_alemao": "zona_norte",
		"tijuca":          "zona_norte",
		"lapa":            "centro",
		"saara":           "centro",
		"cinelandia":      "centro",
		"porto_maravilha": "centro",
		"barra_da_tijuca": "zona_oeste",
		"jacarepagua":     "zona_oeste",
		"campo_grande":    "zona_oeste",
		"santa_cruz":      "zona_oeste",
	}

	zoneID, exists := zoneMap[subZoneID]
	if !exists {
		return "Ei, essa subzona não existe! 🗺️\n\n" +
			"*Zonas disponíveis:*\n\n" +
			"• *Zona Sul:* Copacabana, Ipanema, Leblon, Vidigal 🌊\n" +
			"• *Zona Norte:* Madureira, Méier, Complexo do Alemão, Tijuca 🏙️\n" +
			"• *Centro:* Lapa, SAARA, Cinelândia, Porto Maravilha 🎭\n" +
			"• *Zona Oeste:* Barra da Tijuca, Jacarepaguá, Campo Grande, Santa Cruz 🌅\n\n" +
			"Use: */mover [subzona]*\n" +
			"Exemplo: */mover ipanema*"
	}

	player, err := cm.gameManager.GetPlayer(sender)
	if err != nil {
		return "Ei, você nem começou o jogo ainda! 😅\n\n" +
			"Use */comecar [seu nome]* pra começar sua jornada!"
	}

	if player.CurrentCharacter == nil {
		return "Você ainda não escolheu um personagem! 🤔\n\n" +
			"Use */personagens* pra ver quem você pode ser!"
	}

	err = cm.gameManager.MovePlayer(sender, zoneID, subZoneID)
	if err != nil {
		return fmt.Sprintf("Ops! Não deu pra mudar de lugar: %v 😱\n\n"+
			"Tente de novo ou escolha outro lugar!", err)
	}

	// Get the display name without underscores
	displayName := strings.Title(strings.ReplaceAll(subZoneID, "_", " "))

	// Location-specific messages with multiple options
	locationMessages := map[string][]string{
		"campo_grande": {
			"Você chegou em *Campo Grande*... que calor da porra! 🌡️🔥\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Campo Grande*... onde o ar-condicionado é artigo de luxo! ❄️💸\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Campo Grande*... terra do calor infernal e do suor eterno! 🔥💦\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Campo Grande*... onde até o ventilador pede arrego! 💨😓\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Campo Grande*... onde o sol é mais forte que sua vontade de trabalhar! ☀️😅\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Campo Grande*... onde até o termômetro desiste de medir! 🌡️🤯\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"lapa": {
			"Você chegou na *Lapa*... só tem malandro e pivete aqui, fica ligado! 🎭👀\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Lapa*... onde todo mundo é artista, menos os artistas! 🎨🎭\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Lapa*... terra do samba, da cerveja e da ressaca! 🍺🎵\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Lapa*... onde todo mundo tem uma história pra contar, mas ninguém acredita! 📖🤥\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Lapa*... onde até o mendigo tem mais estilo que você! 👔🎩\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Lapa*... onde a noite é mais movimentada que o dia! 🌙🎉\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"copacabana": {
			"Você chegou em *Copacabana*... cuidado com os gringos e os preços! 💸🌍\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Copacabana*... onde todo mundo é turista, menos os turistas! 🧳👀\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Copacabana*... terra do biquíni fio dental e do dinheiro curto! 👙💸\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Copacabana*... onde até o picolé é importado! 🍦🌍\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Copacabana*... onde todo mundo é rico, menos você! 💰😅\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Copacabana*... onde até o mendigo fala inglês! 🗣️🌍\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"ipanema": {
			"Você chegou em *Ipanema*... onde todo mundo é rico, menos você! 💰😅\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Ipanema*... onde até o cachorro tem pedigree! 🐕👑\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Ipanema*... terra do suco detox e do saldo negativo! 🥤💸\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Ipanema*... onde todo mundo é influencer, menos os influencers! 📱🎭\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Ipanema*... onde até o pão é artesanal! 🥖👨‍🍳\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Ipanema*... onde todo mundo tem iate, menos você! ⛵😅\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"leblon": {
			"Você chegou no *Leblon*... tá vendo aquela mansão? Não é sua! 🏰😅\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Leblon*... onde até o lixo é gourmet! 🗑️👨‍🍳\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Leblon*... terra do suco verde e do cartão vermelho! 💳🥬\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Leblon*... onde todo mundo tem helicóptero, menos você! 🚁😅\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Leblon*... onde até o mendigo tem conta no exterior! 🌍💰\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Leblon*... onde todo mundo é VIP, menos você! 🎫😅\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"vidigal": {
			"Você chegou no *Vidigal*... subiu o morro, agora aguenta! ⛰️💪\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Vidigal*... onde todo mundo é guerreiro! ⚔️🛡️\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Vidigal*... terra do funk e da vista privilegiada! 🎵🌅\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Vidigal*... onde todo mundo tem história pra contar! 📖🎭\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Vidigal*... onde até o cachorro é valente! 🐕💪\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Vidigal*... onde todo mundo é família! 👨‍👩‍👧‍👦❤️\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"madureira": {
			"Você chegou em *Madureira*... terra do samba e do pagode! 🎵💃\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Madureira*... onde todo mundo é bamba! 🕺🎭\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Madureira*... terra do feijão com arroz e do samba no pé! 🍚💃\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Madureira*... onde todo mundo tem ginga! 💃🕺\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Madureira*... onde até o cachorro samba! 🐕💃\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Madureira*... onde todo mundo é bamba do samba! 🎭🎵\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"meier": {
			"Você chegou no *Méier*... onde todo mundo tem um primo que conhece alguém! 🤝👥\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Méier*... terra do cafezinho e da fofoca! ☕🗣️\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Méier*... onde todo mundo é parente! 👨‍👩‍👧‍👦❤️\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Méier*... onde até o cachorro tem QI! 🧠🐕\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Méier*... onde todo mundo tem um jeitinho! 🎭🤝\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Méier*... onde até o mendigo tem networking! 🤝👔\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"complexo_alemao": {
			"Você chegou no *Complexo do Alemão*... fica esperto e não vacila! 🚨👀\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Complexo do Alemão*... onde todo mundo é guerreiro! ⚔️🛡️\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Complexo do Alemão*... terra do funk e da coragem! 🎵💪\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Complexo do Alemão*... onde todo mundo tem história! 📖🎭\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Complexo do Alemão*... onde até o cachorro é chapa quente! 🐕💪\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Complexo do Alemão*... onde todo mundo é família! 👨‍👩‍👧‍👦❤️\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"tijuca": {
			"Você chegou na *Tijuca*... onde todo mundo é formado e desempregado! 🎓😅\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Tijuca*... terra do diploma e do Uber! 🚗🎓\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Tijuca*... onde todo mundo tem currículo! 📄👔\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Tijuca*... onde até o mendigo tem MBA! 🎓👨‍🎓\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Tijuca*... onde todo mundo é especialista! 🧠👨‍💼\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Tijuca*... onde até o cachorro tem LinkedIn! 💼🐕\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"saara": {
			"Você chegou no *SAARA*... onde tudo é barato, menos o que você quer! 💰😅\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *SAARA*... terra da pechincha e do desconto! 🛍️💸\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *SAARA*... onde todo mundo é vendedor! 🏪👨‍💼\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *SAARA*... onde até o mendigo tem loja! 🏬👨‍💼\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *SAARA*... onde todo mundo tem preço! 💵💰\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *SAARA*... onde até o cachorro faz propaganda! 🐕📢\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"cinelandia": {
			"Você chegou na *Cinelândia*... onde todo mundo é ator, menos os atores! 🎬🎭\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Cinelândia*... terra do teatro e do desemprego! 🎭😅\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Cinelândia*... onde todo mundo tem talento! 🎨🎭\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Cinelândia*... onde até o mendigo tem Oscar! 🏆🎭\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Cinelândia*... onde todo mundo é estrela! ⭐🎭\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Cinelândia*... onde até o cachorro tem agente! 🎭🐕\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"porto_maravilha": {
			"Você chegou no *Porto Maravilha*... onde tudo é novo, menos o preço! 🏗️💸\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Porto Maravilha*... terra da gentrificação e do aluguel caro! 💸🏢\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Porto Maravilha*... onde todo mundo é hipster! 🧔🎨\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Porto Maravilha*... onde até o mendigo tem bike! 🚲👨‍💼\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Porto Maravilha*... onde todo mundo é moderno! 🏢🎨\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou no *Porto Maravilha*... onde até o cachorro tem café artesanal! ☕🐕\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"barra_da_tijuca": {
			"Você chegou na *Barra*... onde todo mundo tem carro, menos você! 🚗😅\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Barra*... terra do trânsito e do condomínio fechado! 🏘️🚗\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Barra*... onde todo mundo tem piscina! 🏊🏠\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Barra*... onde até o mendigo tem carro importado! 🚘👨‍💼\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Barra*... onde todo mundo é playboy! 🏄👨‍💼\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou na *Barra*... onde até o cachorro tem coleira de ouro! 🐕💰\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"jacarepagua": {
			"Você chegou em *Jacarepaguá*... onde todo mundo é do Flamengo! 🔴⚫\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Jacarepaguá*... terra do samba e do futebol! ⚽🎵\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Jacarepaguá*... onde todo mundo é rubro-negro! 🔴⚫\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Jacarepaguá*... onde até o mendigo tem camisa do Flamengo! 👕🔴\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Jacarepaguá*... onde todo mundo é Mengão! 🏆🔴\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Jacarepaguá*... onde até o cachorro é flamenguista! 🐕🔴\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
		"santa_cruz": {
			"Você chegou em *Santa Cruz*... onde todo mundo tem um tio que trabalha na fábrica! 🏭👨‍🏭\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Santa Cruz*... terra da indústria e do churrasco! 🍖🏭\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Santa Cruz*... onde todo mundo tem emprego! 💼👨‍💼\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Santa Cruz*... onde até o mendigo tem carteira assinada! 📄👨‍💼\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Santa Cruz*... onde todo mundo é operário! 👷🏭\n\nBem-vindo ao seu novo cantinho! 🏠✨",
			"Você chegou em *Santa Cruz*... onde até o cachorro tem crachá! 🐕👨‍💼\n\nBem-vindo ao seu novo cantinho! 🏠✨",
		},
	}

	// Get the messages for this location, or use a default one
	messages, exists := locationMessages[subZoneID]
	if !exists {
		return fmt.Sprintf("Você chegou em %s ‍♂️\n\nBem-vindo ao seu novo cantinho! 🏠", displayName)
	}

	// Select a random message
	rand.Seed(time.Now().UnixNano())
	message := messages[rand.Intn(len(messages))]

	return message
}

// sendResponse sends a response message
func (cm *ClientManager) sendResponse(phoneNumber string, targetJID waTypes.JID, message string) (string, error) {
	client, exists := cm.GetClient(phoneNumber)
	if !exists {
		return "", fmt.Errorf("client not found for phone number: %s", phoneNumber)
	}

	// Create message
	msg := &waProto.Message{
		Conversation: proto.String(message),
	}

	// Send message
	response, err := client.SendMessage(context.Background(), targetJID, msg)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	return response.ID, nil
}

// cleanCommand normalizes and cleans a command string
func cleanCommand(command string) string {
	// Convert to lowercase
	command = strings.ToLower(command)

	// Remove extra whitespace
	command = strings.TrimSpace(command)

	// Only clean accents for non-event commands
	if !strings.HasPrefix(command, "/") {
		// Remove accents (simplified approach)
		command = strings.ReplaceAll(command, "á", "a")
		command = strings.ReplaceAll(command, "à", "a")
		command = strings.ReplaceAll(command, "â", "a")
		command = strings.ReplaceAll(command, "ã", "a")
		command = strings.ReplaceAll(command, "é", "e")
		command = strings.ReplaceAll(command, "ê", "e")
		command = strings.ReplaceAll(command, "í", "i")
		command = strings.ReplaceAll(command, "ó", "o")
		command = strings.ReplaceAll(command, "ô", "o")
		command = strings.ReplaceAll(command, "õ", "o")
		command = strings.ReplaceAll(command, "ú", "u")
		command = strings.ReplaceAll(command, "ç", "c")
	}

	return command
}

// parseJID converts a string to a WhatsApp JID
func parseJID(jidString string) (waTypes.JID, error) {
	if !strings.ContainsRune(jidString, '@') {
		// Assume this is a phone number, add WhatsApp suffix
		jidString = jidString + "@s.whatsapp.net"
	}

	return waTypes.ParseJID(jidString)
}

func (cm *ClientManager) handleCharactersCommand() string {
	characters := cm.gameManager.GetAvailableCharacters()

	response := "🎭 *PERSONAGENS DISPONÍVEIS* 🎭\n\n"

	for i, char := range characters {
		emoji := getCharacterEmoji(char.Name)

		response += fmt.Sprintf("%d. *%s* %s\n", i+1, char.Name, emoji)
		response += fmt.Sprintf("   Carisma: %d 🎭\n", char.Carisma)
		response += fmt.Sprintf("   Proficiência: %d 🧠\n", char.Proficiencia)
		response += fmt.Sprintf("   Rede: %d 🤝\n", char.Rede)
		response += fmt.Sprintf("   Moralidade: %d 👼\n", char.Moralidade)
		response += fmt.Sprintf("   Resiliência: %d 🥊\n\n", char.Resiliencia)
	}

	response += "Para escolher um personagem, digite: */escolher [número]* 🎯"

	return response
}

// Helper function to get character-specific emoji
func getCharacterEmoji(name string) string {
	switch name {
	case "Coach Motivacional":
		return "💪"
	case "Dono da Boca":
		return "💰"
	case "Engenheiro Público":
		return "🏗️"
	case "Estudante da UERJ":
		return "📚"
	case "Filhinho de Papai":
		return "👶"
	case "Fogueteiro":
		return "🚀"
	case "Nerd Hacker":
		return "💻"
	case "Influencer de Nicho":
		return "📱"
	case "Motoboy":
		return "🏍️"
	case "Músico Independente":
		return "🎸"
	case "Policial Militar":
		return "👮"
	case "Surfista Carioca":
		return "🏄"
	default:
		return "🎭"
	}
}

func (cm *ClientManager) handleChooseCommand(sender, command string) string {
	parts := strings.Fields(command)
	if len(parts) < 2 {
		return "Ei, você esqueceu de escolher um número! 🤔\n\n" +
			"Use: */escolher [número]*"
	}

	index, err := strconv.Atoi(parts[1])
	if err != nil {
		return "Ei, isso não é um número válido! 🧐\n\n" +
			"Use um número da lista de personagens!"
	}

	characters := cm.gameManager.GetAvailableCharacters()
	if index < 1 || index > len(characters) {
		return fmt.Sprintf("Ei, esse número não existe! 😅\n\n"+
			"Escolha entre *1* e *%d*!", len(characters))
	}

	character := characters[index-1]
	err = cm.gameManager.SelectCharacter(sender, character.ID)
	if err != nil {
		if err.Error() == "player already has character" {
			return "Ei, você já escolheu um personagem! 😅\n\n" +
				"Se quiser mudar, vai ter que começar de novo!"
		}
		return fmt.Sprintf("Ops! Algo deu errado: %s 😱", err.Error())
	}

	emoji := getCharacterEmoji(character.Name)

	return fmt.Sprintf("🎉 *PARABÉNS!* Você agora é *%s* %s\n\n"+
		"*Seus atributos:*\n"+
		"Carisma: %d 🎭\n"+
		"Proficiência: %d 🧠\n"+
		"Rede: %d 🤝\n"+
		"Moralidade: %d 👼\n"+
		"Resiliência: %d 🥊\n\n"+
		"Você acorda em *Copacabana* 🌊 com R$ 100,00 💰 e 0 XP ⭐\n\n"+
		"Digite */ajuda* para ver os comandos disponíveis! 🎮",
		character.Name, emoji,
		character.Carisma, character.Proficiencia, character.Rede,
		character.Moralidade, character.Resiliencia)
}

// Helper function to check if an action is available in a zone
func isActionAvailable(zoneID, subZoneID, actionID string) bool {
	// Get available actions for the sub-zone
	availableActions := getAvailableActions(zoneID, subZoneID)

	// Check if the action is in the list
	for _, action := range availableActions {
		if action == actionID {
			return true
		}
	}
	return false
}

// Helper function to get available actions for a zone
func getAvailableActions(zoneID, subZoneID string) []string {
	// This is a simplified version - in a real implementation, you would
	// get this from your game state or configuration
	switch zoneID {
	case "zona_sul":
		switch subZoneID {
		case "copacabana", "ipanema", "leblon":
			return []string{"trabalhar", "relaxar", "curtir", "dormir", "networking", "treinar", "meditar", "empreender"}
		case "vidigal":
			return []string{"trabalhar", "relaxar", "curtir", "dormir", "ajudar", "treinar"}
		}
	case "zona_norte":
		switch subZoneID {
		case "madureira", "meier", "tijuca":
			return []string{"trabalhar", "estudar", "relaxar", "curtir", "dormir", "networking", "treinar", "empreender"}
		case "complexo_alemao":
			return []string{"trabalhar", "estudar", "relaxar", "dormir", "ajudar"}
		}
	case "centro":
		switch subZoneID {
		case "lapa":
			return []string{"trabalhar", "curtir", "dormir", "networking", "empreender"}
		case "saara":
			return []string{"trabalhar", "relaxar", "dormir", "empreender"}
		case "cinelandia":
			return []string{"trabalhar", "estudar", "curtir", "dormir", "networking", "ajudar"}
		case "porto_maravilha":
			return []string{"trabalhar", "estudar", "relaxar", "curtir", "dormir", "networking", "empreender"}
		}
	case "zona_oeste":
		switch subZoneID {
		case "barra_da_tijuca":
			return []string{"trabalhar", "estudar", "relaxar", "curtir", "dormir", "networking", "treinar", "meditar", "empreender"}
		case "jacarepagua", "campo_grande":
			return []string{"trabalhar", "estudar", "relaxar", "dormir", "ajudar", "treinar", "empreender"}
		case "santa_cruz":
			return []string{"trabalhar", "estudar", "relaxar", "dormir", "ajudar"}
		}
	}
	return []string{}
}

// GetBotPhoneNumber returns the phone number of the bot's WhatsApp account
func (cm *ClientManager) GetBotPhoneNumber() (string, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	// Get the first connected client
	for phoneNumber, clientInfo := range cm.clients {
		if clientInfo.Client.IsConnected() && clientInfo.Client.IsLoggedIn() {
			return phoneNumber, nil
		}
	}

	return "", fmt.Errorf("no connected bot client found")
}

// SendMessage implements the game.MessageSender interface
func (cm *ClientManager) SendMessage(phoneNumber, recipient, message string) (string, error) {
	return cm.SendTextMessage(phoneNumber, recipient, message)
}

// Helper function to get subzones for a zone
func getSubzonesForZone(zoneID string) string {
	switch zoneID {
	case "zona_sul":
		return "• Copacabana 🌊\n• Ipanema 🌊\n• Leblon 🌊\n• Vidigal 🌊"
	case "zona_norte":
		return "• Madureira 🏙️\n• Méier 🏙️\n• Complexo do Alemão 🏙️\n• Tijuca 🏙️"
	case "centro":
		return "• Lapa 🎭\n• SAARA 🎭\n• Cinelândia 🎭\n• Porto Maravilha 🎭"
	case "zona_oeste":
		return "• Barra da Tijuca 🌅\n• Jacarepaguá 🌅\n• Campo Grande 🌅\n• Santa Cruz 🌅"
	default:
		return ""
	}
}

// Helper function to normalize subzone names
func normalizeSubzoneName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Replace spaces with underscores
	name = strings.ReplaceAll(name, " ", "_")

	// Remove accents and special characters
	replacements := map[string]string{
		"á": "a", "à": "a", "â": "a", "ã": "a",
		"é": "e", "è": "e", "ê": "e",
		"í": "i", "ì": "i", "î": "i",
		"ó": "o", "ò": "o", "ô": "o", "õ": "o",
		"ú": "u", "ù": "u", "û": "u",
		"ç":  "c",
		"do": "do", // Keep "do" as is
		"da": "da", // Keep "da" as is
	}

	for old, new := range replacements {
		name = strings.ReplaceAll(name, old, new)
	}

	// Special cases for specific subzones
	specialCases := map[string]string{
		"complexo_do_alemao": "complexo_alemao",
		"barra_da_tijuca":    "barra_da_tijuca",
		"porto_maravilha":    "porto_maravilha",
	}

	if normalized, exists := specialCases[name]; exists {
		return normalized
	}

	return name
}
