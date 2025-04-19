package whatsapp

import (
	"context"
	"fmt"
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

	// Parse recipient JID
	recipientJID, err := parseJID(recipient)
	if err != nil {
		return "", err
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
			client = clientInfo.Client
			break
		}
		cm.mutex.RUnlock()

		if client == nil {
			cm.logger.Error("No client available to send response")
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
	// Clean and normalize command
	command = cleanCommand(command)

	// Check if command starts with '/'
	if !strings.HasPrefix(command, "/") {
		return "Comandos devem começar com '/'. Digite '/ajuda' para ver os comandos disponíveis."
	}

	// Remove the '/' prefix
	command = strings.TrimPrefix(command, "/")

	// Check if this is a help command
	if command == "ajuda" || command == "help" {
		return cm.handleHelpCommand()
	}

	// Check if this is a move command
	if strings.HasPrefix(command, "mover") {
		return cm.handleMoveCommand(sender, command)
	}

	// Check if this is a registration command
	if strings.HasPrefix(command, "comecar") || strings.HasPrefix(command, "começar") || strings.HasPrefix(command, "iniciar") {
		return cm.handleRegistrationCommand(sender, command)
	}

	// Check if this is a character selection command
	if strings.HasPrefix(command, "escolher") {
		return cm.handleCharacterSelectionCommand(sender, command)
	}

	// Check if this is a status command
	if command == "status" {
		return cm.handleStatusCommand(sender)
	}

	// Check if this is a characters list command
	if command == "personagens" {
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
		return cm.handleActionCommand(sender, command)
	}

	// Check if this is an event response
	if strings.HasPrefix(command, "a") ||
		strings.HasPrefix(command, "b") ||
		strings.HasPrefix(command, "c") ||
		strings.HasPrefix(command, "d") {
		return cm.handleEventResponseCommand(sender, command)
	}

	// Unknown command
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

	player, err := cm.gameManager.RegisterPlayer(sender, playerName)
	if err != nil {
		if err.Error() == "player already registered" {
			return "Calma aí, você já está no jogo! 😅\n\n" +
				"Digite */status* para ver sua situação atual."
		}
		return fmt.Sprintf("Ops! Algo deu errado: %s 😱", err.Error())
	}

	return fmt.Sprintf("E aí, %s! Bem-vindo ao *VIDA LOKA STRATEGY*! 🎮\n\n"+
		"Agora você pode escolher seu personagem.\n\n"+
		"Digite */personagens* para ver as opções disponíveis. ��", player.Name)
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
	// Get available actions
	actions, err := cm.gameManager.GetAvailableActions(sender)
	if err != nil {
		if err.Error() == "player not found" {
			return "Ei, você nem começou o jogo ainda! 😅\nUse /comecar [seu nome] pra começar sua jornada!"
		}
		if err.Error() == "player has no character selected" {
			return "Você ainda não escolheu um personagem! 🤔\nUse /personagens pra ver quem você pode ser!"
		}
		return fmt.Sprintf("Ops! Algo deu errado: %s 😱", err.Error())
	}

	// Find matching action
	var actionID string
	for _, action := range actions {
		if strings.HasPrefix(command, action.Name) {
			actionID = action.ID
			break
		}
	}

	// Get player's current location
	player, err := cm.gameManager.GetPlayer(sender)
	if err != nil {
		return "Você precisa começar o jogo primeiro! Use /comecar [seu nome]"
	}

	// Check if action is available in current zone
	if actionID == "" || !isActionAvailable(player.CurrentZone, player.CurrentSubZone, actionID) {
		// Get available actions for current location
		availableActions := getAvailableActions(player.CurrentZone, player.CurrentSubZone)
		actionList := strings.Join(availableActions, ", ")

		return fmt.Sprintf("❌ *Ação não disponível em %s!*\n\n"+
			"*Ações disponíveis aqui:*\n%s\n\n"+
			"Use */mover [subzona]* para ir para outro lugar! 🏃‍♂️",
			player.CurrentSubZone, actionList)
	}

	// Perform action
	outcome, err := cm.gameManager.PerformAction(sender, actionID)
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

	// Check if there's a follow-up event
	if outcome.NextEventID != "" {
		response += "\n\nAlgo interessante aconteceu! 🎭\n"
		response += "Responda com /a, /b, /c ou /d para ver o que acontece! 🎲"
	}

	return response
}

// handleEventResponseCommand processes player responses to events
func (cm *ClientManager) handleEventResponseCommand(sender, command string) string {
	// Clean command
	command = cleanCommand(command)

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

	// Get the last event from player's decision history
	if len(player.DecisionHistory) == 0 {
		return "Você não tem nenhum evento pendente! 🎭\n\n" +
			"Continue explorando o mundo para encontrar eventos!"
	}

	lastDecision := player.DecisionHistory[len(player.DecisionHistory)-1]
	if !strings.HasPrefix(lastDecision.EventID, "event_") {
		return "Você não tem nenhum evento pendente! 🎭\n\n" +
			"Continue explorando o mundo para encontrar eventos!"
	}

	// Process event choice
	eventID := strings.TrimPrefix(lastDecision.EventID, "event_")
	outcome, err := cm.gameManager.ProcessEventChoice(sender, eventID, command)
	if err != nil {
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

	// Check if there's a follow-up event
	if outcome.NextEventID != "" {
		response += "\nAlgo interessante aconteceu! 🎭\n"
		response += "Responda com /a, /b, /c ou /d para ver o que acontece! 🎲"
	}

	return response
}

// handleCharactersListCommand returns the list of available characters
func (cm *ClientManager) handleCharactersListCommand() string {
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

	subZoneID := strings.ToLower(parts[1])

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

	return fmt.Sprintf("Você chegou em *%s* 🏃‍♂️\n\n"+
		"Bem-vindo ao seu novo cantinho! 🏠", strings.Title(subZoneID))
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

// SendMessage implements the game.MessageSender interface
func (cm *ClientManager) SendMessage(phoneNumber, recipient, message string) (string, error) {
	return cm.SendTextMessage(phoneNumber, recipient, message)
}
