package game

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/user/vida-loka-strategy/config"
)

func TestRegisterPlayer(t *testing.T) {
	// Setup
	cfg := config.DefaultConfig()
	gameManager := NewGameManager(cfg)

	// Test case 1: Register new player
	player, err := gameManager.RegisterPlayer("5521999999999", "Test Player")
	assert.NoError(t, err)
	assert.NotNil(t, player)
	assert.Equal(t, "Test Player", player.Name)
	assert.Equal(t, "5521999999999", player.PhoneNumber)
	assert.Equal(t, cfg.Game.DefaultXP, player.XP)
	assert.Equal(t, cfg.Game.DefaultMoney, player.Money)
	assert.Equal(t, cfg.Game.DefaultInfluence, player.Influence)
	assert.Equal(t, "active", player.Status)
	assert.Equal(t, 0, player.Stress)
	assert.Len(t, player.DecisionHistory, 0)

	// Test case 2: Register duplicate player
	_, err = gameManager.RegisterPlayer("5521999999999", "Duplicate Player")
	assert.Error(t, err)
	assert.Equal(t, "player already registered", err.Error())

	// Test case 3: Get registered player
	retrievedPlayer, err := gameManager.GetPlayer("5521999999999")
	assert.NoError(t, err)
	assert.Equal(t, player.ID, retrievedPlayer.ID)
	assert.Equal(t, "Test Player", retrievedPlayer.Name)
}

func TestSelectCharacter(t *testing.T) {
	// Setup
	cfg := config.DefaultConfig()
	gameManager := NewGameManager(cfg)
	
	// Create test character
	character := &Character{
		ID:          "test_character",
		Name:        "Test Character",
		Type:        "Test",
		Description: "Test character for unit tests",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Carisma:     3,
		Proficiencia: 4,
		Rede:        2,
		Moralidade:  5,
		Resiliencia: 3,
	}
	
	// Load character into game manager
	gameManager.LoadCharacters([]*Character{character})
	
	// Register player
	player, err := gameManager.RegisterPlayer("5521999999999", "Test Player")
	assert.NoError(t, err)
	assert.NotNil(t, player)
	
	// Test case 1: Select character for player
	err = gameManager.SelectCharacter("5521999999999", "test_character")
	assert.NoError(t, err)
	
	// Verify character was assigned
	player, err = gameManager.GetPlayer("5521999999999")
	assert.NoError(t, err)
	assert.NotNil(t, player.CurrentCharacter)
	assert.Equal(t, "test_character", player.CurrentCharacter.ID)
	assert.Equal(t, "Test Character", player.CurrentCharacter.Name)
	
	// Test case 2: Select non-existent character
	err = gameManager.SelectCharacter("5521999999999", "non_existent_character")
	assert.Error(t, err)
	assert.Equal(t, "character not found", err.Error())
	
	// Test case 3: Select character for non-existent player
	err = gameManager.SelectCharacter("non_existent_player", "test_character")
	assert.Error(t, err)
	assert.Equal(t, "player not found", err.Error())
}

func TestPerformAction(t *testing.T) {
	// Setup
	cfg := config.DefaultConfig()
	gameManager := NewGameManager(cfg)
	
	// Create test character
	character := &Character{
		ID:          "test_character",
		Name:        "Test Character",
		Type:        "Test",
		Description: "Test character for unit tests",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Carisma:     3,
		Proficiencia: 4,
		Rede:        2,
		Moralidade:  5,
		Resiliencia: 3,
		FavoriteActions: []string{"test_action"},
	}
	
	// Create test action
	action := &Action{
		ID:          "test_action",
		Name:        "test_action",
		Description: "Test action for unit tests",
		BaseOutcome: Outcome{
			Description:     "Test outcome",
			XPChange:        10,
			MoneyChange:     50.0,
			InfluenceChange: 5,
			StressChange:    10,
		},
		BonusAttribute: "proficiencia",
		EffectiveZones: []string{"test_zone"},
	}
	
	// Create test zone
	zone := &Zone{
		ID:          "test_zone",
		Name:        "Test Zone",
		Description: "Test zone for unit tests",
		SubZones: []SubZone{
			{
				ID:          "test_subzone",
				Name:        "Test SubZone",
				Description: "Test subzone for unit tests",
				RiskLevel:   5,
				RewardMultiplier: 100,
				AvailableActions: []string{"test_action"},
			},
		},
		RiskLevel:       5,
		RewardMultiplier: 100,
		CommonCharacters: []string{"test_character"},
	}
	
	// Load test data into game manager
	gameManager.LoadCharacters([]*Character{character})
	gameManager.LoadActions([]*Action{action})
	gameManager.LoadZones([]*Zone{zone})
	
	// Register and setup player
	player, err := gameManager.RegisterPlayer("5521999999999", "Test Player")
	assert.NoError(t, err)
	assert.NotNil(t, player)
	
	err = gameManager.SelectCharacter("5521999999999", "test_character")
	assert.NoError(t, err)
	
	// Set player location
	player.Zone = "test_zone"
	player.SubZone = "test_subzone"
	
	// Test case 1: Perform valid action
	outcome, err := gameManager.PerformAction("5521999999999", "test_action")
	assert.NoError(t, err)
	assert.NotNil(t, outcome)
	
	// Verify outcome effects
	player, err = gameManager.GetPlayer("5521999999999")
	assert.NoError(t, err)
	assert.Greater(t, player.XP, cfg.Game.DefaultXP)
	assert.Greater(t, player.Money, cfg.Game.DefaultMoney)
	assert.Greater(t, player.Influence, cfg.Game.DefaultInfluence)
	assert.Greater(t, player.Stress, 0)
	assert.Len(t, player.DecisionHistory, 1)
	
	// Test case 2: Perform action with no character
	newPlayer, err := gameManager.RegisterPlayer("5521888888888", "No Character Player")
	assert.NoError(t, err)
	assert.NotNil(t, newPlayer)
	
	_, err = gameManager.PerformAction("5521888888888", "test_action")
	assert.Error(t, err)
	assert.Equal(t, "player has no character selected", err.Error())
	
	// Test case 3: Perform invalid action
	_, err = gameManager.PerformAction("5521999999999", "invalid_action")
	assert.Error(t, err)
	assert.Equal(t, "action not found", err.Error())
}

func TestGenerateEvent(t *testing.T) {
	// Setup
	cfg := config.DefaultConfig()
	gameManager := NewGameManager(cfg)
	
	// Create test character
	character := &Character{
		ID:          "test_character",
		Name:        "Test Character",
		Type:        "Test",
		Description: "Test character for unit tests",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Carisma:     3,
		Proficiencia: 4,
		Rede:        2,
		Moralidade:  5,
		Resiliencia: 3,
	}
	
	// Create test event
	event := &Event{
		ID:          "test_event",
		Title:       "Test Event",
		Description: "Test event for unit tests",
		CreatedAt:   time.Now(),
		MinXP:       0,
		MinMoney:    0,
		MinInfluence: 0,
		RequiredZone: []string{"test_zone"},
		Options: []EventOption{
			{
				ID:          "test_option",
				Description: "Test option",
				RequiredAttribute: "proficiencia",
				DifficultyLevel: 5,
				SuccessOutcome: Outcome{
					Description:     "Success outcome",
					XPChange:        10,
					MoneyChange:     50.0,
					InfluenceChange: 5,
					StressChange:    -5,
				},
				FailureOutcome: Outcome{
					Description:     "Failure outcome",
					XPChange:        5,
					MoneyChange:     -20.0,
					InfluenceChange: -2,
					StressChange:    10,
				},
			},
		},
		Type: "regular",
	}
	
	// Load test data into game manager
	gameManager.LoadCharacters([]*Character{character})
	gameManager.LoadEvents([]*Event{event})
	
	// Register and setup player
	player, err := gameManager.RegisterPlayer("5521999999999", "Test Player")
	assert.NoError(t, err)
	assert.NotNil(t, player)
	
	err = gameManager.SelectCharacter("5521999999999", "test_character")
	assert.NoError(t, err)
	
	// Set player location
	player.Zone = "test_zone"
	
	// Test case 1: Generate event
	generatedEvent, err := gameManager.GenerateEvent("5521999999999")
	assert.NoError(t, err)
	assert.NotNil(t, generatedEvent)
	assert.Equal(t, "test_event", generatedEvent.ID)
	
	// Test case 2: Generate event for player with no character
	newPlayer, err := gameManager.RegisterPlayer("5521888888888", "No Character Player")
	assert.NoError(t, err)
	assert.NotNil(t, newPlayer)
	
	_, err = gameManager.GenerateEvent("5521888888888")
	assert.Error(t, err)
	assert.Equal(t, "player has no character selected", err.Error())
	
	// Test case 3: Generate event for non-existent player
	_, err = gameManager.GenerateEvent("non_existent_player")
	assert.Error(t, err)
	assert.Equal(t, "player not found", err.Error())
}

func TestProcessEventChoice(t *testing.T) {
	// Setup
	cfg := config.DefaultConfig()
	gameManager := NewGameManager(cfg)
	
	// Create test character
	character := &Character{
		ID:          "test_character",
		Name:        "Test Character",
		Type:        "Test",
		Description: "Test character for unit tests",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Carisma:     3,
		Proficiencia: 4,
		Rede:        2,
		Moralidade:  5,
		Resiliencia: 3,
	}
	
	// Create test event
	event := &Event{
		ID:          "test_event",
		Title:       "Test Event",
		Description: "Test event for unit tests",
		CreatedAt:   time.Now(),
		MinXP:       0,
		MinMoney:    0,
		MinInfluence: 0,
		RequiredZone: []string{"test_zone"},
		Options: []EventOption{
			{
				ID:          "test_option",
				Description: "Test option",
				RequiredAttribute: "proficiencia",
				DifficultyLevel: 5,
				SuccessOutcome: Outcome{
					Description:     "Success outcome",
					XPChange:        10,
					MoneyChange:     50.0,
					InfluenceChange: 5,
					StressChange:    -5,
					NewZone:         "new_zone",
					NewSubZone:      "new_subzone",
				},
				FailureOutcome: Outcome{
					Description:     "Failure outcome",
					XPChange:        5,
					MoneyChange:     -20.0,
					InfluenceChange: -2,
					StressChange:    10,
				},
			},
		},
		Type: "regular",
	}
	
	// Load test data into game manager
	gameManager.LoadCharacters([]*Character{character})
	gameManager.LoadEvents([]*Event{event})
	
	// Register and setup player
	player, err := gameManager.RegisterPlayer("5521999999999", "Test Player")
	assert.NoError(t, err)
	assert.NotNil(t, player)
	
	err = gameManager.SelectCharacter("5521999999999", "test_character")
	assert.NoError(t, err)
	
	// Set initial player state
	initialXP := player.XP
	initialMoney := player.Money
	initialInfluence := player.Influence
	initialStress := player.Stress
	
	// Test case 1: Process event choice
	outcome, err := gameManager.ProcessEventChoice("5521999999999", "test_event", "test_option")
	assert.NoError(t, err)
	assert.NotNil(t, outcome)
	
	// Verify player state changes
	player, err = gameManager.GetPlayer("5521999999999")
	assert.NoError(t, err)
	assert.NotEqual(t, initialXP, player.XP)
	assert.NotEqual(t, initialMoney, player.Money)
	assert.NotEqual(t, initialInfluence, player.Influence)
	assert.NotEqual(t, initialStress, player.Stress)
	assert.Len(t, player.DecisionHistory, 1)
	
	// Check if location was updated (for success outcome)
	if outcome.NewZone != "" {
		assert.Equal(t, outcome.NewZone, player.Zone)
	}
	if outcome.NewSubZone != "" {
		assert.Equal(t, outcome.NewSubZone, player.SubZone)
	}
	
	// Test case 2: Process event choice for non-existent event
	_, err = gameManager.ProcessEventChoice("5521999999999", "non_existent_event", "test_option")
	assert.Error(t, err)
	assert.Equal(t, "event not found", err.Error())
	
	// Test case 3: Process non-existent option
	_, err = gameManager.ProcessEventChoice("5521999999999", "test_event", "non_existent_option")
	assert.Error(t, err)
	assert.Equal(t, "option not found", err.Error())
}

func TestGetPlayerStatus(t *testing.T) {
	// Setup
	cfg := config.DefaultConfig()
	gameManager := NewGameManager(cfg)
	
	// Create test character
	character := &Character{
		ID:          "test_character",
		Name:        "Test Character",
		Type:        "Test",
		Description: "Test character for unit tests",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Carisma:     3,
		Proficiencia: 4,
		Rede:        2,
		Moralidade:  5,
		Resiliencia: 3,
	}
	
	// Create test zone
	zone := &Zone{
		ID:          "test_zone",
		Name:        "Test Zone",
		Description: "Test zone for unit tests",
		SubZones: []SubZone{
			{
				ID:          "test_subzone",
				Name:        "Test SubZone",
				Description: "Test subzone for unit tests",
				RiskLevel:   5,
				RewardMultiplier: 100,
				AvailableActions: []string{"test_action"},
			},
		},
		RiskLevel:       5,
		RewardMultiplier: 100,
		CommonCharacters: []string{"test_character"},
	}
	
	// Load test data into game manager
	gameManager.LoadCharacters([]*Character{character})
	gameManager.LoadZones([]*Zone{zone})
	
	// Register and setup player
	player, err := gameManager.RegisterPlayer("5521999999999", "Test Player")
	assert.NoError(t, err)
	assert.NotNil(t, player)
	
	// Test case 1: Get status for player with no character
	_, err = gameManager.GetPlayerStatus("5521999999999")
	assert.Error(t, err)
	assert.Equal(t, "player has no character selected", err.Error())
	
	// Select character and set location
	err = gameManager.SelectCharacter("5521999999999", "test_character")
	assert.NoError(t, err)
	
	player.Zone = "test_zone"
	player.SubZone = "test_subzone"
	
	// Test case 2: Get status for player with character
	status, err := gameManager.GetPlayerStatus("5521999999999")
	assert.NoError(t, err)
	assert.NotNil(t, status)
	
	// Verify status fields
	assert.Equal(t, "Test Player", status["name"])
	assert.Equal(t, "Test Character", status["character"])
	assert.Equal(t, "Test", status["character_type"])
	assert.Equal(t, player.XP, status["xp"])
	assert.Equal(t, player.Money, status["money"])
	assert.Equal(t, player.Influence, status["influence"])
	assert.Equal(t, player.Stress, status["stress"])
	assert.Contains(t, status["location"], "Test Zone")
	
	// Verify attributes
	attributes, ok := status["attributes"].(map[string]int)
	assert.True(t, ok)
	assert.Equal(t, character.Carisma, attributes["carisma"])
	assert.Equal(t, character.Proficiencia, attributes["proficiencia"])
	assert.Equal(t, character.Rede, attributes["rede"])
	assert.Equal(t, character.Moralidade, attributes["moralidade"])
	assert.Equal(t, character.Resiliencia, attributes["resiliencia"])
	
	// Test case 3: Get status for non-existent player
	_, err = gameManager.GetPlayerStatus("non_existent_player")
	assert.Error(t, err)
	assert.Equal(t, "player not found", err.Error())
}
