# VIDA LOKA STRATEGIA

<p align="center">
  <img src="docs/images/vida-loka-logo.png" alt="VIDA LOKA STRATEGIA Graffiti Banner" width="600"/>
</p>

<p align="center">
  <a href="#about">About</a> •
  <a href="#features">Features</a> •
  <a href="#architecture">Architecture</a> •
  <a href="#installation">Installation</a> •
  <a href="#configuration">Configuration</a> •
  <a href="#deployment">Deployment</a> •
  <a href="#api">API</a> •
  <a href="#contributing">Contributing</a> •
  <a href="#license">License</a>
</p>

## About

**VIDA LOKA STRATEGIA** is a WhatsApp-based RPG game server that simulates urban Brazilian life with strategic elements and social commentary. Players navigate the complex social landscape of Rio de Janeiro through text-based interactions, making decisions that affect their character's development and story progression.

Built with **Go** and the [whatsmeow](https://github.com/tulir/whatsmeow) library, this server provides a complete gaming experience through WhatsApp, allowing players to create characters, perform actions, and participate in events entirely through messaging.

## 🎓 Sociological Foundation

The game design and narrative structure are grounded in contemporary and classical sociological thought, aiming to reflect and question real-life structures:

- **Erving Goffman**: Concepts of social roles and performative identity are embedded in how players assume roles in different zones and interactions.
- **Pierre Bourdieu**: The economy of symbolic, social, cultural, and economic capital is gamified as players accrue influence, XP, and money, each impacting social mobility and decision-making.
- **Gilberto Velho**: Deeply influenced by his urban anthropology and analysis of Brazilian ambivalence, the game reflects contradictory norms, informality, and negotiation of values in peripheral settings.
- **Intersectionality**: Race, class, geography, and opportunity are key systemic forces present in the game's moral dilemmas and character trajectories. These are not just themes, but embedded in game mechanics and missions.

---

<p align="center">
  <a href="#about">About</a> •
  <a href="#features">Features</a> •
  <a href="#architecture">Architecture</a> •
  <a href="#installation">Installation</a> •
  <a href="#configuration">Configuration</a> •
  <a href="#deployment">Deployment</a> •
  <a href="#api">API</a> •
  <a href="#contributing">Contributing</a> •
  <a href="#license">License</a>
</p>

## Development

This project was created using [Manus.im](https://manus.im/), an AI-powered development platform. The initial prompt used to generate the codebase can be found in [prompt_v0.md](prompt_v0.md).

The project was downloaded and further developed using [Cursor.com](https://cursor.com/), a modern IDE with AI-powered development features.

## Features

- **Complete WhatsApp Integration**: Seamless gameplay through WhatsApp messaging using the whatsmeow library
- **Character System**: 12 unique character types with distinct attributes (Carisma, Proficiência, Rede, Moralidade, Resiliência)
- **Attribute-Based Mechanics**: Dice rolling system (1d20 + attribute) for action resolution
- **Resource Management**: XP, money, and influence progression systems
- **Location-Based Gameplay**: 4 main zones of Rio de Janeiro with 16 sub-zones, each with unique characteristics
- **Dynamic Event System**: Regular, mission, and random events with multiple choices and outcomes
- **Persistent Game State**: SQLite database for storing player progress and game state
- **Auto-Pilot System**: AI-driven decision making for inactive players
- **Multi-User Support**: Concurrent gameplay for multiple players

## Architecture

### System Components

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│                 │     │                 │     │                 │
│  WhatsApp API   │◄────┤  Game Server    │◄────┤  Game Engine    │
│  (whatsmeow)    │     │ (HTTP/WebSocket)│     │  (Core Logic)   │
│                 │     │                 │     │                 │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                       │                       │
        │                       │                       │
        ▼                       ▼                       ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│                 │     │                 │     │                 │
│  Session Store  │     │  Game Database  │     │  Content Files  │
│  (WhatsApp)     │     │  (SQLite)       │     │  (JSON)         │
│                 │     │                 │     │                 │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

### Directory Structure

```
vida-loka-strategia/
├── cmd/                    # Command-line applications
│   └── server/             # Main server application
├── internal/               # Private application code
│   ├── game/               # Game logic and mechanics
│   ├── whatsapp/           # WhatsApp integration
│   └── database/           # Database operations
├── pkg/                    # Public libraries that can be used by external applications
├── config/                 # Configuration files
├── assets/                 # Game assets
│   └── data/               # JSON data files for game content
├── docs/                   # Documentation
└── scripts/                # Utility scripts
```

### Technology Stack

- **Backend**: Go 1.21+
- **WhatsApp Integration**: whatsmeow library
- **Database**: SQLite3
- **API**: RESTful HTTP + WebSockets
- **Configuration**: JSON
- **Logging**: zap

## Installation

### Prerequisites

- Go 1.21 or higher
- SQLite3
- Git

### Quick Start

1. Clone the repository:

```bash
git clone https://github.com/yourusername/vida-loka-strategia.git
cd vida-loka-strategia
```

2. Install dependencies:

```bash
go mod download
```

3. Build the server:

```bash
go build -o vida-loka-server ./cmd/server
```

4. Run the server:

```bash
./vida-loka-server --config=./config/config.json
```

### Docker Installation

```bash
# Build the Docker image
docker build -t vida-loka-strategia .

# Run the container
docker run -p 8080:8080 -v $(pwd)/data:/app/data vida-loka-strategia
```

## Configuration

The server is configured using a JSON configuration file. Create a custom configuration by copying the default:

```bash
cp config/default_config.json config/config.json
```

### Configuration Options

```json
{
  "server": {
    "port": "8080",
    "log_level": "info"
  },
  "whatsapp": {
    "store_dir": "./whatsapp-store",
    "client_name": "VIDA LOKA STRATEGIA",
    "auto_reply_timeout": 300
  },
  "game": {
    "default_xp": 0,
    "default_money": 100.0,
    "default_influence": 0,
    "event_interval": 60,
    "random_event_probability": 20
  },
  "database": {
    "path": "./vida-loka.db"
  }
}
```

## Deployment

### Production Deployment

For production environments, we recommend using systemd to manage the server process:

1. Create a systemd service file:

```bash
sudo nano /etc/systemd/system/vida-loka.service
```

2. Add the following content:

```
[Unit]
Description=VIDA LOKA STRATEGIA WhatsApp Game Server
After=network.target

[Service]
User=gameuser
WorkingDirectory=/opt/vida-loka-strategia
ExecStart=/opt/vida-loka-strategia/vida-loka-server --config=/opt/vida-loka-strategia/config/config.json
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

3. Enable and start the service:

```bash
sudo systemctl enable vida-loka.service
sudo systemctl start vida-loka.service
```

### Reverse Proxy Configuration (Nginx)

```nginx
server {
    listen 80;
    server_name game.yourdomain.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }
}
```

## API

The server exposes several HTTP endpoints for monitoring and management:

### Public Endpoints

- `GET /status` - Check server status
- `GET /qr` - Get QR code for WhatsApp Web authentication
- `GET /stats` - Get game statistics

### Admin Endpoints (requires authentication)

- `POST /admin/broadcast` - Send a message to all players
- `POST /admin/event` - Trigger a global event
- `GET /admin/players` - List all players
- `GET /admin/player/:id` - Get player details
- `PUT /admin/player/:id` - Update player data

### WebSocket API

Connect to `/ws` to receive real-time updates about game events and player actions.

## WhatsApp Commands

Players interact with the game using text commands sent via WhatsApp:

| Command | Description |
|---------|-------------|
| `comecar [nome]` | Start the game with your name |
| `escolher [número]` | Select a character type |
| `status` | Show your current status |
| `ajuda` | Show help and command list |
| `trabalhar` | Perform work action |
| `estudar` | Perform study action |
| `relaxar` | Perform relax action |
| `curtir` | Perform enjoy action |
| `dormir` | Perform sleep action |
| `ir [zona]` | Move to a different zone |
| `inventario` | Show inventory |
| `missoes` | Show available missions |

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Guidelines

- Follow Go best practices and coding standards
- Write tests for new features
- Update documentation for significant changes
- Use meaningful commit messages

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgements

- [whatsmeow](https://github.com/tulir/whatsmeow) - Go library for WhatsApp Web
- [chi](https://github.com/go-chi/chi) - Lightweight HTTP router for Go
- [zap](https://github.com/uber-go/zap) - Structured logging for Go

---

<p align="center">
  Made with ❤️ in Rio de Janeiro using <a href="https://manus.im/">Manus.im</a> and <a href="https://cursor.com/">Cursor.com</a>
</p>
