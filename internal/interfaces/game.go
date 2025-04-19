package interfaces

import "github.com/user/vida-loka-strategy/internal/types"

// MessageSender defines the interface for sending messages
type MessageSender interface {
	SendMessage(phoneNumber, recipient, message string) (string, error)
}

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
