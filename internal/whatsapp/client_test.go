package whatsapp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/user/vida-loka-strategy/config"
	"github.com/user/vida-loka-strategy/internal/game"
	"go.uber.org/zap"
)

// Mock GameManager for testing
type MockGameManager struct {
	mock.Mock
}

func (m *MockGameManager) RegisterPlayer(phoneNumber, name string) (*game.Player, error) {
	args := m.Called(phoneNumber, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*game.Player), args.Error(1)
}

func (m *MockGameManager) GetPlayer(phoneNumber string) (*game.Player, error) {
	args := m.Called(phoneNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*game.Player), args.Error(1)
}

func (m *MockGameManager) SelectCharacter(phoneNumber, characterID string) error {
	args := m.Called(phoneNumber, characterID)
	return args.Error(0)
}

func (m *MockGameManager) PerformAction(phoneNumber, actionID string) (*game.Outcome, error) {
	args := m.Called(phoneNumber, actionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*game.Outcome), args.Error(1)
}

func (m *MockGameManager) GenerateEvent(phoneNumber string) (*game.Event, error) {
	args := m.Called(phoneNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*game.Event), args.Error(1)
}

func (m *MockGameManager) ProcessEventChoice(phoneNumber, eventID, optionID string) (*game.Outcome, error) {
	args := m.Called(phoneNumber, eventID, optionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*game.Outcome), args.Error(1)
}

func (m *MockGameManager) GetPlayerStatus(phoneNumber string) (map[string]interface{}, error) {
	args := m.Called(phoneNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *MockGameManager) GetAvailableCharacters() []*game.Character {
	args := m.Called()
	return args.Get(0).([]*game.Character)
}

func (m *MockGameManager) GetAvailableActions(phoneNumber string) ([]*game.Action, error) {
	args := m.Called(phoneNumber)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*game.Action), args.Error(1)
}

func TestProcessGameCommand(t *testing.T) {
	// Setup
	logger, _ := zap.NewDevelopment()
	cfg := config.DefaultConfig()
	mockGameManager := new(MockGameManager)
	
	clientManager := &ClientManager{
		gameManager: mockGameManager,
		config:      cfg,
		logger:      logger,
	}

	// Test case 1: Registration command
	mockGameManager.On("RegisterPlayer", "5521999999999", "Test Player").Return(&game.Player{
		ID:          "test_player_id",
		PhoneNumber: "5521999999999",
		Name:        "Test Player",
		CreatedAt:   time.Now(),
		LastActiveAt: time.Now(),
		XP:          0,
		Money:       100.0,
		Influence:   0,
		Status:      "active",
	}, nil)
	
	mockGameManager.On("GetAvailableCharacters").Return([]*game.Character{
		{
			ID:          "char1",
			Name:        "Character 1",
			Description: "Test character 1",
		},
		{
			ID:          "char2",
			Name:        "Character 2",
			Description: "Test character 2",
		},
	})
	
	response := clientManager.processGameCommand("5521999999999", "comecar Test Player")
	assert.Contains(t, response, "Bem-vindo ao VIDA LOKA STRATEGY, Test Player!")
	assert.Contains(t, response, "Character 1")
	assert.Contains(t, response, "Character 2")
	
	// Test case 2: Character selection command
	mockGameManager.On("SelectCharacter", "5521999999999", "char1").Return(nil)
	
	response = clientManager.processGameCommand("5521999999999", "escolher 1")
	assert.Contains(t, response, "Você escolheu:")
	
	// Test case 3: Status command
	mockStatus := map[string]interface{}{
		"name":          "Test Player",
		"character":     "Character 1",
		"character_type": "Test",
		"xp":            10,
		"money":         150.0,
		"influence":     5,
		"stress":        20,
		"location":      "Test Zone, Test SubZone",
		"attributes": map[string]int{
			"carisma":     3,
			"proficiencia": 4,
			"rede":        2,
			"moralidade":  5,
			"resiliencia": 3,
		},
	}
	
	mockGameManager.On("GetPlayerStatus", "5521999999999").Return(mockStatus, nil)
	
	response = clientManager.processGameCommand("5521999999999", "status")
	assert.Contains(t, response, "STATUS DE Test Player")
	assert.Contains(t, response, "Character 1")
	assert.Contains(t, response, "XP: 10")
	assert.Contains(t, response, "Dinheiro: R$ 150.00")
	
	// Test case 4: Action command
	mockActions := []*game.Action{
		{
			ID:   "trabalhar",
			Name: "trabalhar",
		},
		{
			ID:   "estudar",
			Name: "estudar",
		},
	}
	
	mockOutcome := &game.Outcome{
		Description:     "Você trabalha por algumas horas e recebe seu pagamento.",
		XPChange:        5,
		MoneyChange:     50.0,
		InfluenceChange: 1,
		StressChange:    10,
	}
	
	mockGameManager.On("GetAvailableActions", "5521999999999").Return(mockActions, nil)
	mockGameManager.On("PerformAction", "5521999999999", "trabalhar").Return(mockOutcome, nil)
	
	response = clientManager.processGameCommand("5521999999999", "trabalhar")
	assert.Contains(t, response, "Você trabalha por algumas horas")
	assert.Contains(t, response, "XP: +5")
	assert.Contains(t, response, "Dinheiro: +50.00")
	
	// Test case 5: Help command
	response = clientManager.processGameCommand("5521999999999", "ajuda")
	assert.Contains(t, response, "COMANDOS DO VIDA LOKA STRATEGY")
	assert.Contains(t, response, "comecar [nome]")
	assert.Contains(t, response, "escolher [número]")
	assert.Contains(t, response, "status")
	
	// Test case 6: Unknown command
	response = clientManager.processGameCommand("5521999999999", "comando_desconhecido")
	assert.Contains(t, response, "Comando não reconhecido")
	
	mockGameManager.AssertExpectations(t)
}

func TestHandleRegistrationCommand(t *testing.T) {
	// Setup
	logger, _ := zap.NewDevelopment()
	cfg := config.DefaultConfig()
	mockGameManager := new(MockGameManager)
	
	clientManager := &ClientManager{
		gameManager: mockGameManager,
		config:      cfg,
		logger:      logger,
	}
	
	// Test case 1: Successful registration
	mockGameManager.On("RegisterPlayer", "5521999999999", "Test Player").Return(&game.Player{
		ID:          "test_player_id",
		PhoneNumber: "5521999999999",
		Name:        "Test Player",
	}, nil)
	
	mockGameManager.On("GetAvailableCharacters").Return([]*game.Character{
		{
			ID:          "char1",
			Name:        "Character 1",
			Description: "Test character 1",
		},
	})
	
	response := clientManager.handleRegistrationCommand("5521999999999", "comecar Test Player")
	assert.Contains(t, response, "Bem-vindo ao VIDA LOKA STRATEGY, Test Player!")
	assert.Contains(t, response, "Character 1")
	
	// Test case 2: Missing name
	response = clientManager.handleRegistrationCommand("5521999999999", "comecar")
	assert.Contains(t, response, "Para começar o jogo, digite: comecar [seu nome]")
	
	// Test case 3: Already registered
	mockGameManager.On("RegisterPlayer", "5521888888888", "Already Registered").Return(nil, 
		fmt.Errorf("player already registered"))
	
	response = clientManager.handleRegistrationCommand("5521888888888", "comecar Already Registered")
	assert.Contains(t, response, "Você já está registrado no jogo")
	
	mockGameManager.AssertExpectations(t)
}

func TestHandleCharacterSelectionCommand(t *testing.T) {
	// Setup
	logger, _ := zap.NewDevelopment()
	cfg := config.DefaultConfig()
	mockGameManager := new(MockGameManager)
	
	clientManager := &ClientManager{
		gameManager: mockGameManager,
		config:      cfg,
		logger:      logger,
	}
	
	// Test case 1: Successful character selection
	mockGameManager.On("GetAvailableCharacters").Return([]*game.Character{
		{
			ID:          "char1",
			Name:        "Character 1",
			Description: "Test character 1",
			Carisma:     3,
			Proficiencia: 4,
			Rede:        2,
			Moralidade:  5,
			Resiliencia: 3,
		},
		{
			ID:          "char2",
			Name:        "Character 2",
			Description: "Test character 2",
		},
	})
	
	mockGameManager.On("SelectCharacter", "5521999999999", "char1").Return(nil)
	
	response := clientManager.handleCharacterSelectionCommand("5521999999999", "escolher 1")
	assert.Contains(t, response, "Você escolheu: Character 1")
	assert.Contains(t, response, "Carisma: 3")
	assert.Contains(t, response, "Você acorda em Copacabana")
	
	// Test case 2: Missing number
	response = clientManager.handleCharacterSelectionCommand("5521999999999", "escolher")
	assert.Contains(t, response, "Para escolher um personagem, digite: escolher [número]")
	
	// Test case 3: Invalid number
	response = clientManager.handleCharacterSelectionCommand("5521999999999", "escolher 99")
	assert.Contains(t, response, "Número de personagem inválido")
	
	// Test case 4: Selection error
	mockGameManager.On("SelectCharacter", "5521888888888", "char1").Return(
		fmt.Errorf("player not found"))
	
	response = clientManager.handleCharacterSelectionCommand("5521888888888", "escolher 1")
	assert.Contains(t, response, "Erro ao selecionar personagem")
	
	mockGameManager.AssertExpectations(t)
}

func TestHandleStatusCommand(t *testing.T) {
	// Setup
	logger, _ := zap.NewDevelopment()
	cfg := config.DefaultConfig()
	mockGameManager := new(MockGameManager)
	
	clientManager := &ClientManager{
		gameManager: mockGameManager,
		config:      cfg,
		logger:      logger,
	}
	
	// Test case 1: Successful status retrieval
	mockStatus := map[string]interface{}{
		"name":          "Test Player",
		"character":     "Character 1",
		"character_type": "Test",
		"xp":            10,
		"money":         150.0,
		"influence":     5,
		"stress":        20,
		"location":      "Test Zone, Test SubZone",
		"attributes": map[string]int{
			"carisma":     3,
			"proficiencia": 4,
			"rede":        2,
			"moralidade":  5,
			"resiliencia": 3,
		},
	}
	
	mockGameManager.On("GetPlayerStatus", "5521999999999").Return(mockStatus, nil)
	
	response := clientManager.handleStatusCommand("5521999999999")
	assert.Contains(t, response, "STATUS DE Test Player")
	assert.Contains(t, response, "Character 1 (Test)")
	assert.Contains(t, response, "XP: 10")
	assert.Contains(t, response, "Dinheiro: R$ 150.00")
	assert.Contains(t, response, "Influência: 5")
	assert.Contains(t, response, "Estresse: 20/100")
	assert.Contains(t, response, "Localização: Test Zone, Test SubZone")
	assert.Contains(t, response, "Carisma: 3")
	
	// Test case 2: Player not found
	mockGameManager.On("GetPlayerStatus", "5521888888888").Return(nil, 
		fmt.Errorf("player not found"))
	
	response = clientManager.handleStatusCommand("5521888888888")
	assert.Contains(t, response, "Você ainda não está registrado no jogo")
	
	// Test case 3: No character selected
	mockGameManager.On("GetPlayerStatus", "5521777777777").Return(nil, 
		fmt.Errorf("player has no character selected"))
	
	response = clientManager.handleStatusCommand("5521777777777")
	assert.Contains(t, response, "Você ainda não escolheu um personagem")
	
	mockGameManager.AssertExpectations(t)
}

func TestHandleActionCommand(t *testing.T) {
	// Setup
	logger, _ := zap.NewDevelopment()
	cfg := config.DefaultConfig()
	mockGameManager := new(MockGameManager)
	
	clientManager := &ClientManager{
		gameManager: mockGameManager,
		config:      cfg,
		logger:      logger,
	}
	
	// Test case 1: Successful action execution
	mockActions := []*game.Action{
		{
			ID:   "trabalhar",
			Name: "trabalhar",
		},
		{
			ID:   "estudar",
			Name: "estudar",
		},
	}
	
	mockOutcome := &game.Outcome{
		Description:     "Você trabalha por algumas horas e recebe seu pagamento.",
		XPChange:        5,
		MoneyChange:     50.0,
		InfluenceChange: 1,
		StressChange:    10,
	}
	
	mockGameManager.On("GetAvailableActions", "5521999999999").Return(mockActions, nil)
	mockGameManager.On("PerformAction", "5521999999999", "trabalhar").Return(mockOutcome, nil)
	
	response := clientManager.handleActionCommand("5521999999999", "trabalhar")
	assert.Contains(t, response, "Você trabalha por algumas horas")
	assert.Contains(t, response, "XP: +5")
	assert.Contains(t, response, "Dinheiro: +50.00")
	assert.Contains(t, response, "Influência: +1")
	assert.Contains(t, response, "Estresse: +10")
	
	// Test case 2: Player not found
	mockGameManager.On("GetAvailableActions", "5521888888888").Return(nil, 
		fmt.Errorf("player not found"))
	
	response = clientManager.handleActionCommand("5521888888888", "trabalhar")
	assert.Contains(t, response, "Você ainda não está registrado no jogo")
	
	// Test case 3: Action not available
	mockGameManager.On("GetAvailableActions", "5521777777777").Return(mockActions, nil)
	
	response = clientManager.handleActionCommand("5521777777777", "dormir")
	assert.Contains(t, response, "Essa ação não está disponível na sua localização atual")
	
	mockGameManager.AssertExpectations(t)
}

func TestHandleHelpCommand(t *testing.T) {
	// Setup
	logger, _ := zap.NewDevelopment()
	cfg := config.DefaultConfig()
	mockGameManager := new(MockGameManager)
	
	clientManager := &ClientManager{
		gameManager: mockGameManager,
		config:      cfg,
		logger:      logger,
	}
	
	// Test help command
	response := clientManager.handleHelpCommand()
	assert.Contains(t, response, "COMANDOS DO VIDA LOKA STRATEGY")
	assert.Contains(t, response, "BÁSICOS:")
	assert.Contains(t, response, "comecar [nome]")
	assert.Contains(t, response, "escolher [número]")
	assert.Contains(t, response, "status")
	assert.Contains(t, response, "ajuda")
	assert.Contains(t, response, "AÇÕES:")
	assert.Contains(t, response, "trabalhar")
	assert.Contains(t, response, "estudar")
	assert.Contains(t, response, "relaxar")
	assert.Contains(t, response, "curtir")
	assert.Contains(t, response, "dormir")
	assert.Contains(t, response, "EVENTOS:")
	assert.Contains(t, response, "Responda a eventos com A, B, C ou D")
}
