package game

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/user/vida-loka-strategy/config"
	"github.com/user/vida-loka-strategy/internal/types"
	"go.uber.org/zap"
)

// DataLoader handles loading game data from files
type DataLoader struct {
	basePath string
}

// NewDataLoader creates a new data loader
func NewDataLoader(basePath string) *DataLoader {
	return &DataLoader{
		basePath: basePath,
	}
}

// LoadCharacters loads character definitions from file
func (dl *DataLoader) LoadCharacters() ([]*types.Character, error) {
	path := filepath.Join(dl.basePath, "characters.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read characters file: %w", err)
	}

	var characters []*types.Character
	if err := json.Unmarshal(data, &characters); err != nil {
		return nil, fmt.Errorf("failed to parse characters data: %w", err)
	}

	return characters, nil
}

// LoadEvents loads event definitions from file
func (dl *DataLoader) LoadEvents() ([]*types.Event, error) {
	path := filepath.Join(dl.basePath, "events.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read events file: %w", err)
	}

	var events []*types.Event
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, fmt.Errorf("failed to parse events data: %w", err)
	}

	return events, nil
}

// LoadActions loads action definitions from file
func (dl *DataLoader) LoadActions() ([]*types.Action, error) {
	path := filepath.Join(dl.basePath, "actions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read actions file: %w", err)
	}

	var actions []*types.Action
	if err := json.Unmarshal(data, &actions); err != nil {
		return nil, fmt.Errorf("failed to parse actions data: %w", err)
	}

	return actions, nil
}

// LoadZones loads zone definitions from file
func (dl *DataLoader) LoadZones() ([]*types.Zone, error) {
	path := filepath.Join(dl.basePath, "zones.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read zones file: %w", err)
	}

	var zones []*types.Zone
	if err := json.Unmarshal(data, &zones); err != nil {
		return nil, fmt.Errorf("failed to parse zones data: %w", err)
	}

	return zones, nil
}

// DiceRoller handles dice rolling for the game
type DiceRoller struct {
	rng *rand.Rand
}

// NewDiceRoller creates a new dice roller with a seeded random number generator
func NewDiceRoller() *DiceRoller {
	return &DiceRoller{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Roll rolls a dice with the specified number of sides
func (dr *DiceRoller) Roll(sides int) int {
	return dr.rng.Intn(sides) + 1
}

// RollWithBonus rolls a dice and adds a bonus value
func (dr *DiceRoller) RollWithBonus(sides, bonus int) int {
	return dr.Roll(sides) + bonus
}

// RollD20WithAttribute rolls a d20 and adds an attribute bonus
func (dr *DiceRoller) RollD20WithAttribute(attribute int) int {
	return dr.RollWithBonus(20, attribute)
}

// EventSystem handles scheduling and triggering of game events
type EventSystem struct {
	gameManager *GameManager
	ticker      *time.Ticker
	stopChan    chan struct{}
	logger      *zap.Logger
	diceRoller  *DiceRoller
	config      *config.Config
}

// NewEventSystem creates a new event system
func NewEventSystem(gameManager *GameManager, eventInterval time.Duration, logger *zap.Logger, diceRoller *DiceRoller, config *config.Config) *EventSystem {
	return &EventSystem{
		gameManager: gameManager,
		ticker:      time.NewTicker(eventInterval),
		stopChan:    make(chan struct{}),
		logger:      logger,
		diceRoller:  diceRoller,
		config:      config,
	}
}

// Start begins the event scheduling system
func (es *EventSystem) Start() {
	es.logger.Info("Starting event system",
		zap.Int("event_interval_minutes", es.config.Game.EventInterval),
		zap.Int("event_probability", es.config.Game.RandomEventProbability))

	go func() {
		for {
			select {
			case <-es.ticker.C:
				es.logger.Debug("Event system tick received")
				es.triggerEvents()
			case <-es.stopChan:
				es.logger.Info("Event system received stop signal")
				es.ticker.Stop()
				return
			}
		}
	}()
}

// Stop halts the event scheduling system
func (es *EventSystem) Stop() {
	es.logger.Info("Stopping event system")
	close(es.stopChan)
}

// formatEventMessage formats an event into a WhatsApp message
func formatEventMessage(event *types.Event) string {
	message := fmt.Sprintf("🎭 *EVENTO ALEATÓRIO* 🎭\n\n")
	message += fmt.Sprintf("%s\n\n", event.Description)

	if len(event.Options) > 0 {
		message += "Escolha sua ação:\n"
		for i, option := range event.Options {
			message += fmt.Sprintf("%s. %s\n", string('A'+i), option.Description)
		}
		message += "\nResponda com */a*, */b*, */c* ou */d* para escolher sua ação! 🎲"
	}

	return message
}

// triggerEvents checks for and triggers events for all active players
func (es *EventSystem) triggerEvents() {
	es.logger.Info("Starting event check cycle")

	// Get all players
	players := es.gameManager.GetAllPlayers()
	es.logger.Info("Checking events for players", zap.Int("total_players", len(players)))

	for _, player := range players {
		// Skip inactive players
		if player.Status != "active" {
			es.logger.Info("Skipping inactive player",
				zap.String("phone_number", player.PhoneNumber),
				zap.String("name", player.Name),
				zap.String("status", player.Status),
				zap.String("location", fmt.Sprintf("%s, %s", player.CurrentZone, player.CurrentSubZone)))
			continue
		}

		es.logger.Info("Checking event for player",
			zap.String("phone_number", player.PhoneNumber),
			zap.String("name", player.Name),
			zap.String("location", fmt.Sprintf("%s, %s", player.CurrentZone, player.CurrentSubZone)),
			zap.Int("xp", player.XP),
			zap.Int("money", player.Money),
			zap.Int("influence", player.Influence),
			zap.Int("stress", player.Stress))

		// Roll for event
		roll := es.diceRoller.Roll(100)
		required := es.config.Game.RandomEventProbability
		eventTriggered := roll <= required

		es.logger.Info("Event roll result",
			zap.String("phone_number", player.PhoneNumber),
			zap.String("name", player.Name),
			zap.Int("roll", roll),
			zap.Int("required", required),
			zap.Bool("event_triggered", eventTriggered))

		if eventTriggered {
			es.logger.Info("Event triggered for player",
				zap.String("phone_number", player.PhoneNumber),
				zap.String("name", player.Name))

			// Generate event
			event, err := es.gameManager.TriggerRandomEvent(player.PhoneNumber)
			if err != nil {
				es.logger.Error("Failed to trigger event",
					zap.String("phone_number", player.PhoneNumber),
					zap.String("name", player.Name),
					zap.Error(err))
				continue
			}

			es.logger.Info("Sending event message to player",
				zap.String("phone_number", player.PhoneNumber),
				zap.String("name", player.Name),
				zap.String("event_id", event.ID),
				zap.String("event_description", event.Description),
				zap.Int("options_count", len(event.Options)))

			// Format event message
			message := formatEventMessage(event)

			// Send event message using player's phone number
			if err := es.gameManager.SendMessage(player.PhoneNumber, message); err != nil {
				es.logger.Error("Failed to send event message",
					zap.String("phone_number", player.PhoneNumber),
					zap.String("name", player.Name),
					zap.Error(err))
			}
		} else {
			es.logger.Info("No event triggered for player",
				zap.String("phone_number", player.PhoneNumber),
				zap.String("name", player.Name),
				zap.Int("roll", roll),
				zap.Int("required", required))
		}
	}

	es.logger.Info("Completed event check cycle")
}
