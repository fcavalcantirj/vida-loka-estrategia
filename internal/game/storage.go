package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/user/vida-loka-strategy/internal/types"
)

// GameStateStorage handles persistence of game state
type GameStateStorage struct {
	savePath  string
	stateLock sync.RWMutex
}

// NewGameStateStorage creates a new game state storage
func NewGameStateStorage(savePath string) *GameStateStorage {
	// Create data directory if it doesn't exist
	dir := filepath.Dir(savePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		// If we can't create the directory, we'll just use the default path
		savePath = "./data/game_state.json"
	}

	return &GameStateStorage{
		savePath: savePath,
	}
}

// SaveGameState saves the game state to disk
func (gss *GameStateStorage) SaveGameState(state *types.GameState) error {
	gss.stateLock.Lock()
	defer gss.stateLock.Unlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(gss.savePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal state to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal game state: %w", err)
	}

	// Write to file
	if err := os.WriteFile(gss.savePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write game state: %w", err)
	}

	return nil
}

// LoadGameState loads the game state from disk
func (gss *GameStateStorage) LoadGameState() (*types.GameState, error) {
	gss.stateLock.Lock()
	defer gss.stateLock.Unlock()

	// Check if file exists
	if _, err := os.Stat(gss.savePath); os.IsNotExist(err) {
		// Return empty state if file doesn't exist
		return &types.GameState{
			Players:    make(map[string]*types.Player),
			Characters: make(map[string]*types.Character),
			Events:     make(map[string]*types.Event),
			Actions:    make(map[string]*types.Action),
			Zones:      make(map[string]*types.Zone),
		}, nil
	}

	// Read file
	data, err := os.ReadFile(gss.savePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read game state file: %w", err)
	}

	// Unmarshal JSON
	var state types.GameState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse game state: %w", err)
	}

	// Ensure all maps are initialized
	if state.Players == nil {
		state.Players = make(map[string]*types.Player)
	}
	if state.Characters == nil {
		state.Characters = make(map[string]*types.Character)
	}
	if state.Events == nil {
		state.Events = make(map[string]*types.Event)
	}
	if state.Actions == nil {
		state.Actions = make(map[string]*types.Action)
	}
	if state.Zones == nil {
		state.Zones = make(map[string]*types.Zone)
	}

	// Ensure all zones have initialized subzones
	for _, zone := range state.Zones {
		if zone.SubZones == nil {
			zone.SubZones = make([]types.SubZone, 0)
		}
	}

	return &state, nil
}

// AutoPilotSystem handles automatic decision making for inactive players
type AutoPilotSystem struct {
	gameManager *GameManager
	diceRoller  *DiceRoller
	ticker      *time.Ticker
	stopChan    chan struct{}
}

// NewAutoPilotSystem creates a new auto-pilot system
func NewAutoPilotSystem(gameManager *GameManager, checkInterval time.Duration) *AutoPilotSystem {
	return &AutoPilotSystem{
		gameManager: gameManager,
		diceRoller:  NewDiceRoller(),
		ticker:      time.NewTicker(checkInterval),
		stopChan:    make(chan struct{}),
	}
}

// Start begins the auto-pilot system
func (aps *AutoPilotSystem) Start() {
	go func() {
		for {
			select {
			case <-aps.ticker.C:
				aps.processAutoPilotPlayers()
			case <-aps.stopChan:
				aps.ticker.Stop()
				return
			}
		}
	}()
}

// Stop halts the auto-pilot system
func (aps *AutoPilotSystem) Stop() {
	close(aps.stopChan)
}

// processAutoPilotPlayers handles decision making for players in auto-pilot mode
func (aps *AutoPilotSystem) processAutoPilotPlayers() {
	// This would check all players in auto-pilot mode and make decisions
	// based on their character's attributes and preferences
	// For now, this is a placeholder
}

// DecisionEngine provides AI-based decision making for auto-pilot mode
type DecisionEngine struct {
	diceRoller *DiceRoller
}

// NewDecisionEngine creates a new decision engine
func NewDecisionEngine() *DecisionEngine {
	return &DecisionEngine{
		diceRoller: NewDiceRoller(),
	}
}

// ChooseAction selects an action for a character based on their attributes and preferences
func (de *DecisionEngine) ChooseAction(character *types.Character, availableActions []*types.Action) *types.Action {
	if len(availableActions) == 0 {
		return nil
	}

	// Check for favorite actions first
	var favoriteActions []*types.Action
	for _, action := range availableActions {
		for _, favID := range character.FavoriteActions {
			if action.ID == favID {
				favoriteActions = append(favoriteActions, action)
			}
		}
	}

	// 60% chance to choose a favorite action if available
	if len(favoriteActions) > 0 && de.diceRoller.Roll(100) <= 60 {
		return favoriteActions[de.diceRoller.Roll(len(favoriteActions))-1]
	}

	// Otherwise choose randomly from all available actions
	return availableActions[de.diceRoller.Roll(len(availableActions))-1]
}

// ChooseEventOption selects an option for an event based on character attributes
func (de *DecisionEngine) ChooseEventOption(character *types.Character, event *types.Event) *types.EventOption {
	if len(event.Options) == 0 {
		return nil
	}

	// Calculate scores for each option based on character attributes
	type optionScore struct {
		option *types.EventOption
		score  int
	}

	var scores []optionScore
	for _, option := range event.Options {
		score := 0

		// Check which attribute is required and add corresponding character attribute value
		switch option.RequiredAttribute {
		case "carisma":
			score += character.Carisma * 2
		case "proficiencia":
			score += character.Proficiencia * 2
		case "rede":
			score += character.Rede * 2
		case "moralidade":
			score += character.Moralidade * 2
		case "resiliencia":
			score += character.Resiliencia * 2
		}

		// Adjust score based on difficulty (higher difficulty = lower score)
		score -= option.DifficultyLevel

		// Add some randomness
		score += de.diceRoller.Roll(10)

		scores = append(scores, optionScore{option: &option, score: score})
	}

	// Find option with highest score
	highestScore := -1000
	var bestOption *types.EventOption
	for _, s := range scores {
		if s.score > highestScore {
			highestScore = s.score
			bestOption = s.option
		}
	}

	return bestOption
}
