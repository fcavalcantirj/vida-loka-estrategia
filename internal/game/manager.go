package game

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/user/vida-loka-strategy/config"
	"github.com/user/vida-loka-strategy/internal/interfaces"
	"github.com/user/vida-loka-strategy/internal/types"
	"github.com/user/vida-loka-strategy/internal/whatsapp"
	"go.uber.org/zap"
)

// MessageSender defines the interface for sending messages to players
type MessageSender interface {
	SendMessage(phoneNumber, recipient, message string) (string, error)
}

// GameManager handles the game state and operations
type GameManager struct {
	state         *types.GameState
	stateLock     sync.RWMutex
	storage       *GameStateStorage
	config        config.Config
	Logger        *zap.Logger
	diceRoller    *DiceRoller
	eventSys      *EventSystem
	clientManager *whatsapp.ClientManager
	messageSender interfaces.MessageSender
}

// Ensure GameManager satifies the interfaces.GameManager interface
var _ interfaces.GameManager = (*GameManager)(nil)

// NewGameManager creates a new game manager
func NewGameManager(cfg config.Config) *GameManager {
	// Create storage
	storage := NewGameStateStorage("./data/game_state.json")

	// Try to load existing state
	state, err := storage.LoadGameState()
	if err != nil {
		// If there's an error loading, create a new state
		state = &types.GameState{
			Players:    make(map[string]*types.Player),
			Characters: make(map[string]*types.Character),
			Events:     make(map[string]*types.Event),
			Actions:    make(map[string]*types.Action),
			Zones:      make(map[string]*types.Zone),
		}
	}

	gm := &GameManager{
		state:      state,
		storage:    storage,
		config:     cfg,
		Logger:     zap.NewNop(), // Will be set by the server
		diceRoller: NewDiceRoller(),
	}

	// Initialize event system
	gm.eventSys = NewEventSystem(gm, time.Duration(cfg.Game.EventInterval)*time.Minute)

	return gm
}

// saveState persists the current game state
func (gm *GameManager) saveState() error {
	return gm.storage.SaveGameState(gm.state)
}

// RegisterPlayer adds a new player to the game
func (gm *GameManager) RegisterPlayer(phoneNumber, name string) (*types.Player, error) {
	gm.stateLock.Lock()
	defer gm.stateLock.Unlock()

	// Check if player already exists
	if _, exists := gm.state.Players[phoneNumber]; exists {
		return nil, errors.New("player already registered")
	}

	// Create new player
	player := &types.Player{
		ID:              uuid.New().String(),
		PhoneNumber:     phoneNumber,
		Name:            name,
		CreatedAt:       time.Now(),
		LastActiveAt:    time.Now(),
		XP:              gm.config.Game.DefaultXP,
		Money:           gm.config.Game.DefaultMoney,
		Influence:       gm.config.Game.DefaultInfluence,
		Status:          "active",
		Stress:          0,
		DecisionHistory: make([]types.Decision, 0),
	}

	// Add player to state
	gm.state.Players[phoneNumber] = player

	// Save state
	if err := gm.saveState(); err != nil {
		return nil, fmt.Errorf("failed to save game state: %w", err)
	}

	return player, nil
}

// GetPlayer retrieves a player by phone number
func (gm *GameManager) GetPlayer(phoneNumber string) (*types.Player, error) {
	gm.stateLock.RLock()
	defer gm.stateLock.RUnlock()

	player, exists := gm.state.Players[phoneNumber]
	if !exists {
		return nil, errors.New("player not found")
	}

	return player, nil
}

// SelectCharacter assigns a character to a player
func (gm *GameManager) SelectCharacter(phoneNumber, characterID string) error {
	gm.stateLock.Lock()
	defer gm.stateLock.Unlock()

	// Get player
	player, exists := gm.state.Players[phoneNumber]
	if !exists {
		return errors.New("player not found")
	}

	// Get character
	character, exists := gm.state.Characters[characterID]
	if !exists {
		return errors.New("character not found")
	}

	// Assign character to player
	player.CurrentCharacter = character
	player.LastActiveAt = time.Now()

	// Set initial zone and subzone based on character type
	switch character.Type {
	case "nerd_hacker":
		player.CurrentZone = "copacabana"
		player.CurrentSubZone = "praia"
	case "traficante":
		player.CurrentZone = "centro"
		player.CurrentSubZone = "lapa"
	case "policial":
		player.CurrentZone = "centro"
		player.CurrentSubZone = "delegacia"
	case "artista":
		player.CurrentZone = "ipanema"
		player.CurrentSubZone = "praia"
	case "empresario":
		player.CurrentZone = "barra"
		player.CurrentSubZone = "shopping"
	default:
		player.CurrentZone = "centro"
		player.CurrentSubZone = "lapa"
	}

	// Save state
	if err := gm.saveState(); err != nil {
		return fmt.Errorf("failed to save game state: %w", err)
	}

	return nil
}

// PerformAction executes a common action for a player
func (gm *GameManager) PerformAction(phoneNumber, actionID string) (*types.Outcome, error) {
	gm.stateLock.Lock()
	defer gm.stateLock.Unlock()

	// Get player
	player, exists := gm.state.Players[phoneNumber]
	if !exists {
		return nil, errors.New("player not found")
	}

	// Check if player has a character
	if player.CurrentCharacter == nil {
		return nil, errors.New("player has no character selected")
	}

	// Get action
	action, exists := gm.state.Actions[actionID]
	if !exists {
		return nil, errors.New("action not found")
	}

	// Get zone
	zone, exists := gm.state.Zones[player.CurrentZone]
	if !exists {
		return nil, errors.New("invalid player zone")
	}

	// Find current subzone
	var currentSubZone *types.SubZone
	for _, sz := range zone.SubZones {
		if sz.ID == player.CurrentSubZone {
			currentSubZone = &sz
			break
		}
	}

	if currentSubZone == nil {
		return nil, errors.New("invalid player subzone")
	}

	// Check if action is available in current subzone
	actionAvailable := false
	for _, availableAction := range currentSubZone.AvailableActions {
		if availableAction == actionID {
			actionAvailable = true
			break
		}
	}

	if !actionAvailable {
		return nil, errors.New("action not available in current location")
	}

	// Get attribute value for bonus
	var attributeValue int
	switch action.BonusAttribute {
	case "carisma":
		attributeValue = player.CurrentCharacter.Carisma
	case "proficiencia":
		attributeValue = player.CurrentCharacter.Proficiencia
	case "rede":
		attributeValue = player.CurrentCharacter.Rede
	case "moralidade":
		attributeValue = player.CurrentCharacter.Moralidade
	case "resiliencia":
		attributeValue = player.CurrentCharacter.Resiliencia
	default:
		attributeValue = 0
	}

	// Calculate bonus multiplier (1% per point)
	bonusMultiplier := float64(attributeValue) / 100.0

	// Apply bonus to outcome
	outcome := action.BaseOutcome
	outcome.XPChange = int(float64(outcome.XPChange) * (1 + bonusMultiplier))
	outcome.MoneyChange = int(float64(outcome.MoneyChange) * (1 + bonusMultiplier))
	outcome.InfluenceChange = int(float64(outcome.InfluenceChange) * (1 + bonusMultiplier))

	// Apply zone multiplier
	zoneMultiplier := float64(currentSubZone.RewardMultiplier) / 100.0
	outcome.XPChange = int(float64(outcome.XPChange) * (1 + zoneMultiplier))
	outcome.MoneyChange = int(float64(outcome.MoneyChange) * (1 + zoneMultiplier))
	outcome.InfluenceChange = int(float64(outcome.InfluenceChange) * (1 + zoneMultiplier))

	// Check if action is a favorite
	for _, favAction := range player.CurrentCharacter.FavoriteActions {
		if favAction == actionID {
			// Apply 20% bonus for favorite actions
			outcome.XPChange = int(float64(outcome.XPChange) * 1.2)
			outcome.MoneyChange = int(float64(outcome.MoneyChange) * 1.2)
			outcome.InfluenceChange = int(float64(outcome.InfluenceChange) * 1.2)
			break
		}
	}

	// Apply outcome to player
	player.XP += outcome.XPChange
	player.Money += outcome.MoneyChange
	player.Influence += outcome.InfluenceChange
	player.Stress += outcome.StressChange

	// Ensure stress stays within bounds
	if player.Stress < 0 {
		player.Stress = 0
	}
	if player.Stress > 100 {
		player.Stress = 100
	}

	// Update player's last active time
	player.LastActiveAt = time.Now()

	// Record decision
	decision := types.Decision{
		ID:              uuid.New().String(),
		EventID:         "action_" + actionID,
		Choice:          actionID,
		Timestamp:       time.Now(),
		Outcome:         action.Name,
		XPChange:        outcome.XPChange,
		MoneyChange:     outcome.MoneyChange,
		InfluenceChange: outcome.InfluenceChange,
		StressChange:    outcome.StressChange,
	}
	player.DecisionHistory = append(player.DecisionHistory, decision)

	// Save state
	if err := gm.saveState(); err != nil {
		return nil, fmt.Errorf("failed to save game state: %w", err)
	}

	return &outcome, nil
}

// GenerateEvent creates a new event for a player
func (gm *GameManager) GenerateEvent(phoneNumber string) (*types.Event, error) {
	gm.stateLock.Lock()
	defer gm.stateLock.Unlock()

	// Get player
	player, exists := gm.state.Players[phoneNumber]
	if !exists {
		return nil, errors.New("player not found")
	}

	// Check if player has a character
	if player.CurrentCharacter == nil {
		return nil, errors.New("player has no character selected")
	}

	// Get all events that match player's current state
	var eligibleEvents []*types.Event
	for _, event := range gm.state.Events {
		// Check requirements
		if player.XP < event.MinXP || player.Money < event.MinMoney || player.Influence < event.MinInfluence {
			continue
		}

		// Check zone requirements if specified
		if len(event.RequiredZone) > 0 {
			zoneMatch := false
			for _, zone := range event.RequiredZone {
				if zone == player.CurrentZone {
					zoneMatch = true
					break
				}
			}
			if !zoneMatch {
				continue
			}
		}

		eligibleEvents = append(eligibleEvents, event)
	}

	// If no eligible events, return error
	if len(eligibleEvents) == 0 {
		return nil, errors.New("no eligible events found for player")
	}

	// Select random event
	selectedEvent := eligibleEvents[rand.Intn(len(eligibleEvents))]

	// Update player's last event time
	player.LastEventAt = time.Now()

	// Save state
	if err := gm.saveState(); err != nil {
		return nil, fmt.Errorf("failed to save game state: %w", err)
	}

	return selectedEvent, nil
}

// ProcessEventChoice handles a player's choice in an event
func (gm *GameManager) ProcessEventChoice(phoneNumber, eventID, optionID string) (*types.Outcome, error) {
	gm.stateLock.Lock()
	defer gm.stateLock.Unlock()

	// Get player
	player, exists := gm.state.Players[phoneNumber]
	if !exists {
		return nil, errors.New("player not found")
	}

	// Check if player has a character
	if player.CurrentCharacter == nil {
		return nil, errors.New("player has no character selected")
	}

	// Get event
	event, exists := gm.state.Events[eventID]
	if !exists {
		return nil, errors.New("event not found")
	}

	// Find option
	var selectedOption *types.EventOption
	for _, option := range event.Options {
		if option.ID == optionID {
			selectedOption = &option
			break
		}
	}

	if selectedOption == nil {
		return nil, errors.New("option not found")
	}

	// Determine attribute value for check
	var attributeValue int
	switch selectedOption.RequiredAttribute {
	case "carisma":
		attributeValue = player.CurrentCharacter.Carisma
	case "proficiencia":
		attributeValue = player.CurrentCharacter.Proficiencia
	case "rede":
		attributeValue = player.CurrentCharacter.Rede
	case "moralidade":
		attributeValue = player.CurrentCharacter.Moralidade
	case "resiliencia":
		attributeValue = player.CurrentCharacter.Resiliencia
	default:
		attributeValue = 0
	}

	// Roll dice (1d20 + attribute)
	roll := rand.Intn(20) + 1 + attributeValue

	// Determine outcome
	var outcome types.Outcome
	if roll >= selectedOption.DifficultyLevel {
		outcome = selectedOption.SuccessOutcome
	} else {
		outcome = selectedOption.FailureOutcome
	}

	// Apply outcome to player
	player.XP += outcome.XPChange
	player.Money += outcome.MoneyChange
	player.Influence += outcome.InfluenceChange
	player.Stress += outcome.StressChange

	// Ensure stress stays within bounds
	if player.Stress < 0 {
		player.Stress = 0
	}
	if player.Stress > 100 {
		player.Stress = 100
	}

	// Update location if specified
	if outcome.NewZone != "" {
		player.CurrentZone = outcome.NewZone
	}
	if outcome.NewSubZone != "" {
		player.CurrentSubZone = outcome.NewSubZone
	}

	// Update player's last active time
	player.LastActiveAt = time.Now()

	// Record decision
	decision := types.Decision{
		ID:              uuid.New().String(),
		EventID:         eventID,
		Choice:          optionID,
		Timestamp:       time.Now(),
		Outcome:         fmt.Sprintf("Roll: %d, Required: %d, Success: %t", roll, selectedOption.DifficultyLevel, roll >= selectedOption.DifficultyLevel),
		XPChange:        outcome.XPChange,
		MoneyChange:     outcome.MoneyChange,
		InfluenceChange: outcome.InfluenceChange,
		StressChange:    outcome.StressChange,
	}
	player.DecisionHistory = append(player.DecisionHistory, decision)

	// Save state
	if err := gm.saveState(); err != nil {
		return nil, fmt.Errorf("failed to save game state: %w", err)
	}

	return &outcome, nil
}

// SetPlayerStatus updates a player's status
func (gm *GameManager) SetPlayerStatus(phoneNumber, status string) error {
	gm.stateLock.Lock()
	defer gm.stateLock.Unlock()

	// Get player
	player, exists := gm.state.Players[phoneNumber]
	if !exists {
		return errors.New("player not found")
	}

	// Validate status
	if status != "active" && status != "sleeping" && status != "autopilot" {
		return errors.New("invalid status")
	}

	// Update status
	player.Status = status
	player.LastActiveAt = time.Now()

	// Save state
	if err := gm.saveState(); err != nil {
		return fmt.Errorf("failed to save game state: %w", err)
	}

	return nil
}

// GetPlayerStatus retrieves a player's current status and stats
func (gm *GameManager) GetPlayerStatus(phoneNumber string) (map[string]interface{}, error) {
	gm.stateLock.RLock()
	defer gm.stateLock.RUnlock()

	// Get player
	player, exists := gm.state.Players[phoneNumber]
	if !exists {
		return nil, errors.New("player not found")
	}

	// Check if player has a character
	if player.CurrentCharacter == nil {
		return nil, errors.New("player has no character selected")
	}

	// Get zone info
	zone, exists := gm.state.Zones[player.CurrentZone]
	if !exists {
		return nil, errors.New("invalid player zone")
	}

	// Find current subzone
	var subZoneName string
	for _, sz := range zone.SubZones {
		if sz.ID == player.CurrentSubZone {
			subZoneName = sz.Name
			break
		}
	}

	// Build status response
	status := map[string]interface{}{
		"name":           player.Name,
		"character":      player.CurrentCharacter.Name,
		"character_type": player.CurrentCharacter.Type,
		"xp":             player.XP,
		"money":          player.Money,
		"influence":      player.Influence,
		"stress":         player.Stress,
		"location":       fmt.Sprintf("%s, %s", zone.Name, subZoneName),
		"status":         player.Status,
		"attributes": map[string]int{
			"carisma":      player.CurrentCharacter.Carisma,
			"proficiencia": player.CurrentCharacter.Proficiencia,
			"rede":         player.CurrentCharacter.Rede,
			"moralidade":   player.CurrentCharacter.Moralidade,
			"resiliencia":  player.CurrentCharacter.Resiliencia,
		},
	}

	return status, nil
}

// LoadCharacters loads character definitions into the game state
func (gm *GameManager) LoadCharacters(characters []*types.Character) {
	gm.stateLock.Lock()
	defer gm.stateLock.Unlock()

	for _, character := range characters {
		gm.state.Characters[character.ID] = character
	}
}

// LoadEvents loads event definitions into the game state
func (gm *GameManager) LoadEvents(events []*types.Event) {
	gm.stateLock.Lock()
	defer gm.stateLock.Unlock()

	for _, event := range events {
		gm.state.Events[event.ID] = event
	}
}

// LoadActions loads action definitions into the game state
func (gm *GameManager) LoadActions(actions []*types.Action) {
	gm.stateLock.Lock()
	defer gm.stateLock.Unlock()

	for _, action := range actions {
		gm.state.Actions[action.ID] = action
	}
}

// LoadZones loads zone definitions into the game state
func (gm *GameManager) LoadZones(zones []*types.Zone) {
	gm.stateLock.Lock()
	defer gm.stateLock.Unlock()

	for _, zone := range zones {
		// Create a new zone with the same ID
		newZone := &types.Zone{
			ID:               zone.ID,
			Name:             zone.Name,
			Description:      zone.Description,
			SubZones:         make([]types.SubZone, len(zone.SubZones)),
			RiskLevel:        zone.RiskLevel,
			RewardMultiplier: zone.RewardMultiplier,
			CommonCharacters: zone.CommonCharacters,
		}

		// Copy subzones
		for i, subZone := range zone.SubZones {
			newZone.SubZones[i] = types.SubZone{
				ID:               subZone.ID,
				Name:             subZone.Name,
				Description:      subZone.Description,
				RiskLevel:        subZone.RiskLevel,
				RewardMultiplier: subZone.RewardMultiplier,
				AvailableActions: subZone.AvailableActions,
			}
		}

		// Store in game state
		gm.state.Zones[zone.ID] = newZone
	}

	// Save the updated state
	if err := gm.saveState(); err != nil {
		gm.Logger.Error("Failed to save game state after loading zones", zap.Error(err))
	}
}

// GetAvailableCharacters returns all available characters
func (gm *GameManager) GetAvailableCharacters() []*types.Character {
	gm.stateLock.RLock()
	defer gm.stateLock.RUnlock()

	// Create a slice of character IDs
	ids := make([]string, 0, len(gm.state.Characters))
	for id := range gm.state.Characters {
		ids = append(ids, id)
	}

	// Sort the IDs to ensure consistent order
	sort.Strings(ids)

	// Create a slice of characters in the sorted order
	characters := make([]*types.Character, 0, len(ids))
	for _, id := range ids {
		characters = append(characters, gm.state.Characters[id])
	}

	return characters
}

// GetAvailableActions returns actions available to a player in their current location
func (gm *GameManager) GetAvailableActions(phoneNumber string) ([]*types.Action, error) {
	gm.stateLock.RLock()
	defer gm.stateLock.RUnlock()

	// Get player
	player, exists := gm.state.Players[phoneNumber]
	if !exists {
		return nil, errors.New("player not found")
	}

	// Check if player has a character
	if player.CurrentCharacter == nil {
		return nil, errors.New("player has no character selected")
	}

	// Get zone
	zone, exists := gm.state.Zones[player.CurrentZone]
	if !exists {
		return nil, errors.New("invalid player zone")
	}

	// Find current subzone
	var currentSubZone *types.SubZone
	for _, sz := range zone.SubZones {
		if sz.ID == player.CurrentSubZone {
			currentSubZone = &sz
			break
		}
	}

	if currentSubZone == nil {
		return nil, errors.New("invalid player subzone")
	}

	// Get available actions
	availableActions := make([]*types.Action, 0)
	for _, actionID := range currentSubZone.AvailableActions {
		if action, exists := gm.state.Actions[actionID]; exists {
			availableActions = append(availableActions, action)
		}
	}

	return availableActions, nil
}

// MovePlayer moves a player to a new zone and subzone
func (gm *GameManager) MovePlayer(playerID, zoneID, subZoneID string) error {
	player, err := gm.GetPlayer(playerID)
	if err != nil {
		return err
	}

	zone, exists := gm.state.Zones[zoneID]
	if !exists {
		return fmt.Errorf("zone not found: %s", zoneID)
	}

	var subZone *types.SubZone
	for _, sz := range zone.SubZones {
		if sz.ID == subZoneID {
			subZone = &sz
			break
		}
	}

	if subZone == nil {
		return fmt.Errorf("subzone not found: %s", subZoneID)
	}

	player.CurrentZone = zoneID
	player.CurrentSubZone = subZoneID

	// Save state
	if err := gm.saveState(); err != nil {
		return fmt.Errorf("failed to save game state: %w", err)
	}

	return nil
}

// GetZone retrieves a zone by ID
func (gm *GameManager) GetZone(zoneID string) (*types.Zone, error) {
	gm.stateLock.RLock()
	defer gm.stateLock.RUnlock()

	zone, exists := gm.state.Zones[zoneID]
	if !exists {
		return nil, fmt.Errorf("zone not found: %s", zoneID)
	}

	return zone, nil
}

// GetAllPlayers returns all players in the game
func (gm *GameManager) GetAllPlayers() []*types.Player {
	gm.stateLock.RLock()
	defer gm.stateLock.RUnlock()

	players := make([]*types.Player, 0, len(gm.state.Players))
	for _, player := range gm.state.Players {
		players = append(players, player)
	}
	return players
}

// TriggerRandomEvent triggers a random event for a player
func (gm *GameManager) TriggerRandomEvent(playerID string) (*types.Event, error) {
	gm.stateLock.Lock()
	defer gm.stateLock.Unlock()

	// Get player by ID
	var player *types.Player
	for _, p := range gm.state.Players {
		if p.ID == playerID {
			player = p
			break
		}
	}

	if player == nil {
		return nil, errors.New("player not found")
	}

	// Get available events for player's current zone
	var availableEvents []*types.Event
	for _, event := range gm.state.Events {
		// Check if event is available in player's current zone
		for _, zone := range event.RequiredZone {
			if zone == player.CurrentZone {
				availableEvents = append(availableEvents, event)
				break
			}
		}
	}

	if len(availableEvents) == 0 {
		return nil, errors.New("no events available for current zone")
	}

	// Select random event
	event := availableEvents[gm.diceRoller.Roll(len(availableEvents))-1]
	return event, nil
}

// SetClientManager sets the WhatsApp client manager
func (gm *GameManager) SetClientManager(clientManager *whatsapp.ClientManager) {
	gm.clientManager = clientManager
}

// SetMessageSender sets the message sender
func (gm *GameManager) SetMessageSender(sender interfaces.MessageSender) {
	gm.messageSender = sender
}

// SendMessage sends a message to a player
func (gm *GameManager) SendMessage(playerID string, message string) error {
	if gm.messageSender == nil {
		return fmt.Errorf("message sender not set")
	}

	// Get player by ID first
	var player *types.Player
	for _, p := range gm.state.Players {
		if p.ID == playerID {
			player = p
			break
		}
	}

	// If not found by ID, try phone number
	if player == nil {
		player, _ = gm.state.Players[playerID]
	}

	if player == nil {
		return fmt.Errorf("player not found: %s", playerID)
	}

	// Send message through message sender
	_, err := gm.messageSender.SendMessage(player.PhoneNumber, player.PhoneNumber, message)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// StartEventSystem starts the event system
func (gm *GameManager) StartEventSystem() {
	gm.eventSys.Start()
}

// StopEventSystem stops the event system
func (gm *GameManager) StopEventSystem() {
	gm.eventSys.Stop()
}
