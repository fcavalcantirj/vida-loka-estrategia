# VIDA LOKA STRATEGY - Documentação para Desenvolvedores

## 📋 Visão Geral do Sistema

VIDA LOKA STRATEGY é um jogo de RPG baseado em texto jogado via WhatsApp, implementado em Go. O sistema é composto por vários componentes principais:

1. **Core Game Engine**: Gerencia a lógica do jogo, personagens, eventos e estado
2. **WhatsApp Integration**: Lida com a comunicação via WhatsApp usando a biblioteca whatsmeow
3. **HTTP Server**: Fornece endpoints para gerenciamento de sessões e autenticação QR
4. **Data Storage**: Gerencia a persistência do estado do jogo e dados dos jogadores

## 🏗️ Arquitetura do Sistema

```
vida-loka-strategy/
├── cmd/                    # Pontos de entrada da aplicação
│   └── server/             # Servidor principal
├── internal/               # Código interno da aplicação
│   ├── game/               # Lógica do jogo
│   ├── whatsapp/           # Integração com WhatsApp
│   └── database/           # Acesso a dados
├── pkg/                    # Bibliotecas reutilizáveis
├── config/                 # Configurações
├── assets/                 # Recursos do jogo
│   └── data/               # Dados do jogo (personagens, eventos, etc.)
└── docs/                   # Documentação
```

## 🔧 Componentes Principais

### Game Manager

O `GameManager` é o componente central que gerencia toda a lógica do jogo:

- Registro e gerenciamento de jogadores
- Seleção de personagens
- Execução de ações
- Geração e processamento de eventos
- Cálculo de resultados baseados em atributos e rolagens de dados

```go
// GameManager handles the game state and operations
type GameManager struct {
    state     *GameState
    config    config.Config
    stateLock sync.RWMutex
}
```

### WhatsApp Client Manager

O `ClientManager` gerencia a integração com o WhatsApp:

- Configuração e gerenciamento de clientes WhatsApp
- Processamento de mensagens recebidas
- Interpretação de comandos do jogo
- Envio de respostas formatadas

```go
// ClientManager handles WhatsApp client connections
type ClientManager struct {
    clients     map[string]*ClientInfo
    gameManager *game.GameManager
    config      config.Config
    logger      *zap.Logger
    mutex       sync.RWMutex
}
```

### Modelos de Dados

Os principais modelos de dados incluem:

- **Character**: Define um personagem jogável com atributos e características
- **Player**: Representa um jogador com seu personagem e progresso
- **Event**: Define um evento do jogo com opções e resultados
- **Action**: Representa uma ação que o jogador pode executar
- **Zone**: Define uma região do Rio de Janeiro com subzonas

## 🔄 Fluxo de Execução

1. **Inicialização**:
   - Carregamento de configurações
   - Inicialização do GameManager
   - Carregamento de dados do jogo (personagens, eventos, ações, zonas)
   - Configuração do servidor HTTP
   - Restauração de sessões WhatsApp existentes

2. **Processamento de Mensagens**:
   - Recebimento de mensagem via WhatsApp
   - Extração do conteúdo e remetente
   - Interpretação do comando
   - Execução da lógica de jogo correspondente
   - Envio da resposta formatada

3. **Execução de Ações**:
   - Verificação de disponibilidade da ação
   - Cálculo de bônus baseados em atributos
   - Aplicação de modificadores de zona
   - Atualização do estado do jogador
   - Geração de resposta com resultados

4. **Processamento de Eventos**:
   - Geração de evento baseado no estado do jogador
   - Apresentação de opções ao jogador
   - Recebimento da escolha do jogador
   - Rolagem de dados para determinar sucesso
   - Aplicação do resultado e atualização do estado

## 🔌 Integração com WhatsApp

A integração com WhatsApp é feita usando a biblioteca [whatsmeow](https://github.com/tulir/whatsmeow), que fornece uma API Go para o WhatsApp Web.

### Autenticação

A autenticação é feita via código QR:

1. O servidor gera um código QR usando a API do whatsmeow
2. O usuário escaneia o código com o WhatsApp
3. A sessão é estabelecida e armazenada para uso futuro

### Processamento de Mensagens

As mensagens são processadas através de um handler de eventos:

```go
func (cm *ClientManager) handleWhatsAppEvent(evt interface{}) {
    switch v := evt.(type) {
    case *events.Message:
        cm.handleIncomingMessage(v)
    // Outros tipos de eventos...
    }
}
```

## 🎲 Sistema de Jogo

### Atributos e Verificações

O sistema usa rolagens de dados (d20) + atributos para determinar o sucesso:

```go
// Roll dice (1d20 + attribute)
roll := rand.Intn(20) + 1 + attributeValue

// Determine outcome
var outcome Outcome
if roll >= selectedOption.DifficultyLevel {
    outcome = selectedOption.SuccessOutcome
} else {
    outcome = selectedOption.FailureOutcome
}
```

### Progressão do Jogador

Os jogadores progridem através de:

- **XP**: Ganho principalmente através de estudo e eventos
- **Dinheiro**: Ganho principalmente através de trabalho
- **Influência**: Ganha através de networking e eventos sociais

### Eventos e Missões

Os eventos são categorizados em:

- **Regular**: Eventos comuns do dia a dia
- **Mission**: Eventos que formam uma narrativa contínua
- **Random**: Eventos aleatórios que podem ocorrer a qualquer momento

## 🗄️ Armazenamento de Dados

### Dados do Jogo

Os dados do jogo são armazenados em arquivos JSON:

- `characters.json`: Definições de personagens
- `events.json`: Definições de eventos
- `actions.json`: Definições de ações
- `zones.json`: Definições de zonas e subzonas

### Estado do Jogo

O estado do jogo é persistido em:

- Arquivos SQLite para sessões WhatsApp
- Arquivos JSON para o estado do jogo

## 🚀 Implantação

### Requisitos

- Go 1.21 ou superior
- SQLite
- Acesso à internet para comunicação com WhatsApp

### Configuração

As configurações são definidas em `config/config.json`:

```json
{
  "whatsapp": {
    "store_dir": "./whatsapp-store",
    "client_name": "VIDA LOKA STRATEGY",
    "auto_reply_timeout": 300
  },
  "database": {
    "driver": "sqlite3",
    "dsn": "./vida-loka.db"
  },
  "game": {
    "default_xp": 0,
    "default_money": 100.0,
    "default_influence": 0,
    "event_interval": 60,
    "random_event_probability": 20
  },
  "server": {
    "port": "8080",
    "log_level": "info"
  }
}
```

### Compilação

```bash
go build -o vida-loka-server ./cmd/server
```

### Execução

```bash
./vida-loka-server --config=./config/config.json
```

## 🧪 Testes

O sistema inclui testes unitários e de integração:

- Testes para o GameManager em `internal/game/manager_test.go`
- Testes para o WhatsApp Client em `internal/whatsapp/client_test.go`

Execute os testes com:

```bash
go test ./...
```

## 🔍 Depuração

O sistema usa o pacote `zap` para logging:

```go
logger.Debug("Received message",
    zap.String("content", content),
    zap.String("sender", message.Info.Sender.User),
    zap.String("chat", message.Info.Chat.User))
```

Os níveis de log podem ser configurados em `config.json`.

## 🛠️ Extensão e Personalização

### Adicionando Novos Personagens

Adicione novos personagens em `assets/data/characters.json`:

```json
{
  "id": "novo_personagem",
  "name": "Novo Personagem",
  "type": "Tipo",
  "description": "Descrição do personagem",
  "carisma": 3,
  "proficiencia": 4,
  "rede": 2,
  "moralidade": 5,
  "resiliencia": 3,
  "favorite_actions": ["acao1", "acao2"],
  "natural_predators": ["predador1", "predador2"],
  "evolution_paths": ["caminho1", "caminho2"]
}
```

### Adicionando Novos Eventos

Adicione novos eventos em `assets/data/events.json`:

```json
{
  "id": "novo_evento",
  "title": "Título do Evento",
  "description": "Descrição do evento",
  "min_xp": 10,
  "min_money": 50,
  "min_influence": 0,
  "required_zone": ["zona_sul"],
  "options": [
    {
      "id": "opcao_a",
      "description": "Descrição da opção A",
      "required_attribute": "carisma",
      "difficulty_level": 7,
      "success_outcome": {
        "description": "Resultado de sucesso",
        "xp_change": 10,
        "money_change": 50.0,
        "influence_change": 5,
        "stress_change": -5
      },
      "failure_outcome": {
        "description": "Resultado de falha",
        "xp_change": 5,
        "money_change": -20.0,
        "influence_change": -2,
        "stress_change": 10
      }
    }
    // Mais opções...
  ],
  "type": "regular"
}
```

## 📝 Notas de Implementação

### Concorrência

O sistema usa mutexes para proteger o acesso concorrente ao estado do jogo:

```go
func (gm *GameManager) RegisterPlayer(phoneNumber, name string) (*Player, error) {
    gm.stateLock.Lock()
    defer gm.stateLock.Unlock()
    
    // Lógica de registro...
}
```

### Tratamento de Erros

Os erros são propagados para cima e tratados no nível apropriado:

```go
if err != nil {
    logger.Error("Failed to generate QR code", 
        zap.String("phone_number", req.PhoneNumber),
        zap.Error(err))
    http.Error(w, "Failed to generate QR code", http.StatusInternalServerError)
    return
}
```

### Segurança

- As sessões WhatsApp são armazenadas localmente
- Não há armazenamento de senhas ou dados sensíveis
- O acesso ao servidor HTTP deve ser protegido por HTTPS

## 🔮 Futuras Melhorias

1. **Dashboard Web**: Interface para administração e visualização de estatísticas
2. **Modo Multiplayer**: Missões cooperativas e interações entre jogadores
3. **Economia Dinâmica**: Sistema econômico que responde às ações dos jogadores
4. **Eventos Sazonais**: Eventos especiais baseados em datas e feriados
5. **Sistema de Reputação**: Reputação em diferentes facções e grupos sociais

## 📚 Referências

- [whatsmeow Documentation](https://github.com/tulir/whatsmeow)
- [Go Chi Router](https://github.com/go-chi/chi)
- [Zap Logger](https://github.com/uber-go/zap)
- [SQLite](https://www.sqlite.org/index.html)

## 👥 Contribuição

Para contribuir com o projeto:

1. Fork o repositório
2. Crie uma branch para sua feature (`git checkout -b feature/nova-feature`)
3. Commit suas mudanças (`git commit -m 'Adiciona nova feature'`)
4. Push para a branch (`git push origin feature/nova-feature`)
5. Abra um Pull Request

## 📄 Licença

Este projeto está licenciado sob a licença MIT - veja o arquivo LICENSE para detalhes.
