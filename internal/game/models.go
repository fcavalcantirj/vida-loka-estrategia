package game

import (
	"time"
)

// Character represents a playable character in the game
type Character struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Base attributes
	Carisma      int `json:"carisma"`
	Proficiencia int `json:"proficiencia"`
	Rede         int `json:"rede"`
	Moralidade   int `json:"moralidade"`
	Resiliencia  int `json:"resiliencia"`

	// Favorite actions that give bonuses
	FavoriteActions []string `json:"favorite_actions"`

	// Natural predators (character types that have advantage)
	NaturalPredators []string `json:"natural_predators"`

	// Unique evolution paths
	EvolutionPaths []string `json:"evolution_paths"`
}

// Player represents a game player
type Player struct {
	ID               string     `json:"id"`
	PhoneNumber      string     `json:"phone_number"`
	Name             string     `json:"name"`
	CreatedAt        time.Time  `json:"created_at"`
	LastActiveAt     time.Time  `json:"last_active_at"`
	XP               int        `json:"xp"`
	Money            int        `json:"money"`
	Influence        int        `json:"influence"`
	Status           string     `json:"status"`
	Stress           int        `json:"stress"`
	CurrentCharacter *Character `json:"current_character"`
	CurrentZone      string     `json:"current_zone"`
	CurrentSubZone   string     `json:"current_sub_zone"`
	LastEventAt      time.Time  `json:"last_event_at"`
	DecisionHistory  []Decision `json:"decision_history"`
}

// Decision represents a choice made by a player
type Decision struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	Choice    string    `json:"choice"`
	Timestamp time.Time `json:"timestamp"`
	Outcome   string    `json:"outcome"`

	// Changes resulting from decision
	XPChange        int `json:"xp_change"`
	MoneyChange     int `json:"money_change"`
	InfluenceChange int `json:"influence_change"`
	StressChange    int `json:"stress_change"`
}

// Event represents a game event presented to the player
type Event struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`

	// Event requirements
	MinXP        int      `json:"min_xp"`
	MinMoney     int      `json:"min_money"`
	MinInfluence int      `json:"min_influence"`
	RequiredZone []string `json:"required_zone"`

	// Event options
	Options []EventOption `json:"options"`

	// Event type
	Type string `json:"type"` // regular, random, mission
}

// EventOption represents a choice in an event
type EventOption struct {
	ID          string `json:"id"`
	Description string `json:"description"`

	// Attribute checks
	RequiredAttribute string `json:"required_attribute"`
	DifficultyLevel   int    `json:"difficulty_level"`

	// Outcomes
	SuccessOutcome Outcome `json:"success_outcome"`
	FailureOutcome Outcome `json:"failure_outcome"`
}

// Outcome represents the result of a decision
type Outcome struct {
	Description     string `json:"description"`
	XPChange        int    `json:"xp_change"`
	MoneyChange     int    `json:"money_change"`
	InfluenceChange int    `json:"influence_change"`
	StressChange    int    `json:"stress_change"`

	// Location changes
	NewZone    string `json:"new_zone,omitempty"`
	NewSubZone string `json:"new_sub_zone,omitempty"`

	// Follow-up event (if any)
	NextEventID string `json:"next_event_id,omitempty"`
}

// Action represents a common action a player can take
type Action struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	// Base outcomes
	BaseOutcome Outcome `json:"base_outcome"`

	// Attribute that gives bonus
	BonusAttribute string `json:"bonus_attribute"`

	// Zones where this action is more effective
	EffectiveZones []string `json:"effective_zones"`
}

// Zone represents a geographic area in the game
type Zone struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	// Sub-zones within this zone
	SubZones []SubZone `json:"sub_zones"`

	// Zone characteristics
	RiskLevel        int `json:"risk_level"`        // 1-10
	RewardMultiplier int `json:"reward_multiplier"` // percentage

	// Common character types in this zone
	CommonCharacters []string `json:"common_characters"`
}

// SubZone represents a specific location within a zone
type SubZone struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	// Sub-zone characteristics
	RiskLevel        int `json:"risk_level"`        // 1-10
	RewardMultiplier int `json:"reward_multiplier"` // percentage

	// Available actions in this sub-zone
	AvailableActions []string `json:"available_actions"`
}

// GameState represents the overall state of the game
type GameState struct {
	Players map[string]*Player `json:"players"`

	// Available characters
	Characters map[string]*Character `json:"characters"`

	// Available events
	Events map[string]*Event `json:"events"`

	// Available actions
	Actions map[string]*Action `json:"actions"`

	// Available zones
	Zones map[string]*Zone `json:"zones"`
}
