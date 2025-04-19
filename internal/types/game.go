package types

import "time"

// GameState represents the overall state of the game
type GameState struct {
	Players    map[string]*Player    `json:"players"`
	Characters map[string]*Character `json:"characters"`
	Events     map[string]*Event     `json:"events"`
	Actions    map[string]*Action    `json:"actions"`
	Zones      map[string]*Zone      `json:"zones"`
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
	CurrentEvent     *Event     `json:"current_event"`
	DecisionHistory  []Decision `json:"decision_history"`
}

// Character represents a playable character
type Character struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Type             string    `json:"type"`
	Description      string    `json:"description"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Carisma          int       `json:"carisma"`
	Proficiencia     int       `json:"proficiencia"`
	Rede             int       `json:"rede"`
	Moralidade       int       `json:"moralidade"`
	Resiliencia      int       `json:"resiliencia"`
	FavoriteActions  []string  `json:"favorite_actions"`
	NaturalPredators []string  `json:"natural_predators"`
	EvolutionPaths   []string  `json:"evolution_paths"`
}

// Event represents a game event
type Event struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	MinXP        int           `json:"min_xp"`
	MinMoney     int           `json:"min_money"`
	MinInfluence int           `json:"min_influence"`
	RequiredZone []string      `json:"required_zone"`
	Options      []EventOption `json:"options"`
}

// EventOption represents an option in an event
type EventOption struct {
	ID                string  `json:"id"`
	Description       string  `json:"description"`
	RequiredAttribute string  `json:"required_attribute"`
	DifficultyLevel   int     `json:"difficulty_level"`
	SuccessOutcome    Outcome `json:"success_outcome"`
	FailureOutcome    Outcome `json:"failure_outcome"`
}

// Outcome represents the result of an action or event
type Outcome struct {
	Description     string `json:"description"`
	XPChange        int    `json:"xp_change"`
	MoneyChange     int    `json:"money_change"`
	InfluenceChange int    `json:"influence_change"`
	StressChange    int    `json:"stress_change"`
	NewZone         string `json:"new_zone,omitempty"`
	NewSubZone      string `json:"new_sub_zone,omitempty"`
	NextEventID     string `json:"next_event_id,omitempty"`
}

// Action represents a game action
type Action struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	BonusAttribute string  `json:"bonus_attribute"`
	BaseOutcome    Outcome `json:"base_outcome"`
}

// Zone represents a game zone
type Zone struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	SubZones         []SubZone `json:"sub_zones"`
	RiskLevel        int       `json:"risk_level"`
	RewardMultiplier int       `json:"reward_multiplier"`
	CommonCharacters []string  `json:"common_characters"`
}

// SubZone represents a sub-zone within a zone
type SubZone struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	RiskLevel        int      `json:"risk_level"`
	RewardMultiplier int      `json:"reward_multiplier"`
	AvailableActions []string `json:"available_actions"`
}

// Decision represents a player's decision in the game
type Decision struct {
	ID              string    `json:"id"`
	EventID         string    `json:"event_id"`
	Choice          string    `json:"choice"`
	Timestamp       time.Time `json:"timestamp"`
	Outcome         string    `json:"outcome"`
	XPChange        int       `json:"xp_change"`
	MoneyChange     int       `json:"money_change"`
	InfluenceChange int       `json:"influence_change"`
	StressChange    int       `json:"stress_change"`
}
