# 🧠 SUPER PROMPT — VIDA LOKA STRATEGY (JOGO VIA WHATSAPP)

## 🎮 O que é?

VIDA LOKA STRATEGY é um jogo de **estratégia e evolução de personagem**, jogável inteiramente via **WhatsApp**, onde o jogador encarna um perfil da sociedade urbana brasileira — de estudante da UERJ a dono da boca, de coach motivacional a surfista de Ipanema.

É um simulador de vida, com **decisões morais, eventos caóticos, rolagem de dados, e evolução baseada em XP, dinheiro e influência**. Mistura **RPG leve**, **crítica social** e **humor ácido**.

---

## 🧩 Mecânica Geral

- Jogável **via WhatsApp**
- Cada jogador escolhe um **personagem com atributos únicos**
- Recebe mensagens automáticas com situações do cotidiano
- Decide entre opções (A, B, C, D), com base em seus atributos
- Rola dados (ex: `1d20 + Carisma`) para verificar sucesso
- Acumula **XP, dinheiro e influência**
- Pode seguir caminhos legítimos, ilegítimos ou ambíguos
- Não há final fixo: o objetivo é **evoluir e sobreviver**

---

## ⚙️ Tecnologia

### 📡 Comunicação via WhatsApp
- Utiliza [**whatsmeow**](https://github.com/tulir/whatsmeow) para se conectar à API do WhatsApp
- Recebe e envia mensagens de forma programada
- Detecta mensagens recebidas dos usuários e aciona a lógica de jogo

### 💻 Backend
- Feito em **Golang**
- Lida com eventos, decisões, armazenamento de progresso e rolagens
- Modular para permitir expansão futura (ex: dashboard, mapas, ranking)

### 📥 Exemplo de integração com WhatsApp:

> Código baseado no handler do projeto Rei-do-Chat:

🔗 https://raw.githubusercontent.com/Rei-do-chat/whatsapp-handler/refs/heads/main/main.go?token=GHSAT0AAAAAAC6TZ45ALDKEDYBXMKU3WMJQ2ACZL2Q

---

## 🗨️ Como os jogadores interagem?

Os jogadores podem:
- **Enviar mensagens pro bot** (ex: “trabalhar”, “estudar”, “ver status”)
- **Aguardar eventos e responder** quando o servidor envia opções (estilo “escolha A, B ou C”)

Se não responderem, entra um **modo auto-pilot**, que toma decisões baseadas no perfil do personagem.

---

## 🌍 Ambientação: Rio de Janeiro Real

O jogo acontece no Rio, com zonas e subzonas:

- **Zona Sul**: Copacabana, Ipanema, Vidigal…
- **Zona Norte**: Méier, Madureira…
- **Centro**: Saara, Lapa…
- **Zona Oeste**: Bangu, Campo Grande…

Cada região tem:
- Eventos únicos
- Variações de risco/recompensa
- Perfis mais comuns e predadores naturais

---

## 🎓 Fundamentação Sociológica

Baseado nas ideias de:

- **Goffman**: papéis sociais, identidade performática
- **Bourdieu**: capitais econômico, cultural, simbólico, social
- **Gilberto Velho**: antropologia urbana, ambivalência brasileira
- **Interseccionalidade**: raça, classe, geografia, oportunidade

---

## 🎭 Personagens Jogáveis (exemplos)

Cada personagem possui:
- Atributos iniciais
- Ações favoritas
- Caminhos de evolução únicos
- Predadores naturais

Exemplos:
- Estudante da UERJ
- Dono da Boca
- Policial Militar
- Influencer de Nicho
- Fogueteiro
- Nerd Hacker
- Coach Motivacional
- Surfista Carioca
- Engenheiro Público
- Filhinho de Papai
- Motoboy
- Músico Independente

Atributos:
- **Carisma**
- **Proficiência**
- **Rede**
- **Moralidade**
- **Resiliência**

---

## 🔁 Ações Comuns

- **Estudar** – Gera XP, aumenta proficiência
- **Trabalhar** – Pode ser legal ou ilegal, gera grana
- **Relaxar** – Reduz estresse, pode gerar eventos aleatórios
- **Curtir** – Diversão e chance de treta
- **Dormir** – Necessário, pausa o jogo

---

## 🎲 Exemplo de Evento

text
[8:00 AM]
Você acorda em Copacabana com R$ 87,00 e XP 123.
Um conhecido te oferece uma chance de fazer R$ 300 hoje.

Escolha sua ação:
A. Recusar e ir estudar
B. Aceitar e ver no que dá
C. Perguntar se é treta
D. Ir tomar café na padaria e fingir demência


---

## 👥 Solo vs Grupo

- **Solo**: mais introspectivo, decisões controladas
- **Grupo**: missões cooperativas, traições, repasses, criação de facções

---

## 📊 Painel Externo (Dashboard)

- Mapa da cidade com eventos e zonas de risco
- Ranking por XP, dinheiro e influência
- Histórico de decisões
- Perfis em destaque

---

## ✨ Estilo

- Humor ácido, mas respeitoso
- Crítica social com leveza
- Linguagem informal, inteligente e acessível
- RPG leve com profundidade estratégica

---

## 📦 Pronto para desenvolvimento

Esse prompt descreve:
- As regras do jogo
- A estrutura de interação
- A tecnologia por trás (whatsmeow + Golang)
- O tom, o cenário e o modelo sociológico
- E um exemplo funcional de integração real

---