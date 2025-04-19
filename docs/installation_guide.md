# VIDA LOKA STRATEGIA - Guia de Instalação

Este guia fornece instruções passo a passo para instalar e executar o VIDA LOKA STRATEGIA, um jogo de RPG baseado em WhatsApp.

## Requisitos do Sistema

- Go 1.21 ou superior
- SQLite3
- Acesso à internet para comunicação com WhatsApp
- Pelo menos 1GB de RAM e 500MB de espaço em disco

## Instalação

### 1. Clonar o Repositório

```bash
git clone https://github.com/user/vida-loka-strategia.git
cd vida-loka-strategia
```

### 2. Instalar Dependências

```bash
go mod download
```

### 3. Configurar o Ambiente

Crie um arquivo de configuração personalizado (opcional):

```bash
cp config/default_config.json config/config.json
```

Edite `config/config.json` conforme necessário para ajustar:
- Diretório de armazenamento do WhatsApp
- Configurações do banco de dados
- Parâmetros do jogo
- Porta do servidor

### 4. Compilar o Projeto

```bash
go build -o vida-loka-server ./cmd/server
```

## Execução

### 1. Iniciar o Servidor

```bash
./vida-loka-server --config=./config/config.json
```

Por padrão, o servidor será iniciado na porta 8080.

### 2. Autenticar o WhatsApp

1. Acesse `http://localhost:8080/qr` em seu navegador
2. Escaneie o código QR com seu WhatsApp
3. Aguarde a confirmação de conexão

### 3. Verificar o Status

Acesse `http://localhost:8080/status` para verificar se o servidor está funcionando corretamente.

## Configuração Avançada

### Configuração do WhatsApp

Para personalizar a integração com WhatsApp, edite a seção `whatsapp` no arquivo de configuração:

```json
"whatsapp": {
  "store_dir": "./whatsapp-store",
  "client_name": "VIDA LOKA STRATEGIA",
  "auto_reply_timeout": 300
}
```

### Configuração do Jogo

Para ajustar os parâmetros do jogo, edite a seção `game`:

```json
"game": {
  "default_xp": 0,
  "default_money": 100.0,
  "default_influence": 0,
  "event_interval": 60,
  "random_event_probability": 20
}
```

### Configuração do Servidor

Para ajustar as configurações do servidor HTTP, edite a seção `server`:

```json
"server": {
  "port": "8080",
  "log_level": "info"
}
```

## Solução de Problemas

### Problemas de Conexão com WhatsApp

Se o servidor não conseguir se conectar ao WhatsApp:

1. Verifique se o código QR foi escaneado corretamente
2. Certifique-se de que o WhatsApp no seu telefone está conectado à internet
3. Exclua o diretório `whatsapp-store` e tente novamente

```bash
rm -rf ./whatsapp-store
./vida-loka-server --config=./config/config.json
```

### Erros de Banco de Dados

Se ocorrerem erros relacionados ao banco de dados:

1. Verifique se o SQLite está instalado corretamente
2. Certifique-se de que o diretório tem permissões de escrita
3. Tente recriar o banco de dados:

```bash
rm -f ./vida-loka.db
./vida-loka-server --config=./config/config.json
```

### Logs e Depuração

Para aumentar o nível de detalhamento dos logs, altere `log_level` para `debug` no arquivo de configuração:

```json
"server": {
  "port": "8080",
  "log_level": "debug"
}
```

## Implantação em Produção

Para implantar o servidor em um ambiente de produção:

### 1. Configurar Systemd (Linux)

Crie um arquivo de serviço systemd:

```bash
sudo nano /etc/systemd/system/vida-loka.service
```

Adicione o seguinte conteúdo:

```
[Unit]
Description=VIDA LOKA STRATEGIA WhatsApp Game Server
After=network.target

[Service]
User=ubuntu
WorkingDirectory=/path/to/vida-loka-strategia
ExecStart=/path/to/vida-loka-strategia/vida-loka-server --config=/path/to/vida-loka-strategy/config/config.json
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

Ative e inicie o serviço:

```bash
sudo systemctl enable vida-loka.service
sudo systemctl start vida-loka.service
```

### 2. Configurar Proxy Reverso (Nginx)

Instale o Nginx:

```bash
sudo apt-get install nginx
```

Configure um site:

```bash
sudo nano /etc/nginx/sites-available/vida-loka
```

Adicione a configuração:

```
server {
    listen 80;
    server_name seu-dominio.com;

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

Ative o site e reinicie o Nginx:

```bash
sudo ln -s /etc/nginx/sites-available/vida-loka /etc/nginx/sites-enabled/
sudo systemctl restart nginx
```

### 3. Configurar HTTPS (Certbot)

Instale o Certbot:

```bash
sudo apt-get install certbot python3-certbot-nginx
```

Obtenha um certificado SSL:

```bash
sudo certbot --nginx -d seu-dominio.com
```

## Backup e Restauração

### Backup

Para fazer backup dos dados do jogo:

```bash
# Backup do banco de dados
cp ./vida-loka.db ./vida-loka.db.backup

# Backup das sessões do WhatsApp
cp -r ./whatsapp-store ./whatsapp-store.backup
```

### Restauração

Para restaurar a partir de um backup:

```bash
# Restaurar banco de dados
cp ./vida-loka.db.backup ./vida-loka.db

# Restaurar sessões do WhatsApp
cp -r ./whatsapp-store.backup ./whatsapp-store
```

## Atualizações

Para atualizar o servidor para uma nova versão:

```bash
# Parar o servidor
sudo systemctl stop vida-loka.service

# Fazer backup
cp ./vida-loka.db ./vida-loka.db.backup
cp -r ./whatsapp-store ./whatsapp-store.backup

# Atualizar o código
git pull

# Recompilar
go build -o vida-loka-server ./cmd/server

# Reiniciar o servidor
sudo systemctl start vida-loka.service
```

## Suporte

Se encontrar problemas durante a instalação ou execução, consulte:

- A documentação do desenvolvedor em `docs/developer_guide.md`
- Os logs do sistema: `journalctl -u vida-loka.service`
- Abra uma issue no repositório do GitHub

---

Para mais informações sobre como jogar, consulte o Manual do Usuário em `docs/user_manual.md`.
