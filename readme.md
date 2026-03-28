# 🛡️ Lattice Privacy Engine

**High-Performance Zero-Trust Gateway for Enterprise LLM Integration.**

Lattice is a lightweight, ultra-fast privacy proxy written in **Go**. It enables corporations to use Public LLMs (OpenAI, Anthropic, Gemini) without ever sending Personally Identifiable Information (PII) to the cloud.

---

## ⚡ Instant Demo

Clone the repo and run the automated demo to see Lattice anonymize real PII in seconds:

```bash
# Linux / macOS / WSL
git clone https://github.com/Karcsihack/lattice-proxy.git
cd lattice-proxy
chmod +x demo.sh && ./demo.sh
```

```powershell
# Windows (PowerShell)
git clone https://github.com/Karcsihack/lattice-proxy.git
cd lattice-proxy
.\demo.ps1
```

Expected console output:

```
[LATTICE] ┌ Texto Original  : Hola, soy Juan Pérez. Mi DNI es 12345678Z y mi correo juan@email.com
[LATTICE] └ Texto a la Nube : Hola, soy [USER_1]. Mi DNI es [ID_NUMBER_2] y mi correo [EMAIL_3]
[LATTICE] ✓ Completado | entidades=3 | latencia_anon=91ms | latencia_total=308ms
```

> No real PII ever reaches the cloud. The upstream API only sees tokens.

---

## 🚀 The Problem: The "Privacy Gap"

Enterprises are blocking AI adoption because of data leakage risks. Sending customer names, emails, or financial data to third-party APIs is a compliance nightmare (GDPR, SOC2, HIPAA).

## ✨ The Solution: Lattice

Lattice sits between your internal users and the AI Provider. It uses **Local NER (Named Entity Recognition)** to intercept, vault, and anonymize sensitive data before it leaves your infrastructure.

### Key Features

- **Zero-Trust Architecture:** Sensitive data stays in your RAM/Redis. Never on the wire.
- **Sub-100ms Latency:** Built with Golang's high-concurrency model for enterprise scale.
- **Plug & Play:** Drop-in replacement for OpenAI SDKs (just change the `BASE_URL`).
- **Local AI Power:** Integrated with Ollama (Llama-3/Mistral) for offline PII detection.

---

## 🛠️ Architecture Flow

1. **Inbound:** User sends prompt with PII -> Lattice detects entities (Name, SSN, Credit Card).
2. **Vaulting:** Lattice stores real data in a local secure Vault and replaces it with unique Tokens.
3. **Cloud Processing:** OpenAI receives the "Clean" prompt (e.g., "Tell me about USER_1's insurance policy").
4. **De-anonymization:** Lattice intercepts the response, replaces Tokens with real data, and delivers it to the user.

---

## ⚡ Quick Start

### 1. Requirements

- Go 1.21+
- Ollama (running `llama3`)
- Docker (Optional)

### 2. Run with Docker

```bash
docker build -t lattice-privacy .
docker run -p 8080:8080 lattice-privacy
```

### 3. Run locally

```bash
# Start Ollama with a local model
ollama pull llama3
ollama serve

# Start Lattice
export OPENAI_API_BASE=https://api.openai.com
./lattice.exe   # Windows
# ./lattice     # Linux / Mac
```

### 4. Test it

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "My name is Carlos García, DNI 12345678Z"}]
  }'
```

Console output:

```
[LATTICE] ┌ Texto Original  : My name is Carlos García, DNI 12345678Z
[LATTICE] └ Texto a la Nube : My name is [USER_1], DNI [ID_NUMBER_2]
[LATTICE] ✓ Completado | entidades=2 | latencia_anon=84ms | latencia_total=310ms
```

---

## ⚙️ Environment Variables

| Variable          | Default                  | Description                  |
| ----------------- | ------------------------ | ---------------------------- |
| `LISTEN_ADDR`     | `:8080`                  | Address Lattice listens on   |
| `OLLAMA_URL`      | `http://localhost:11434` | Local Ollama endpoint        |
| `OLLAMA_MODEL`    | `llama3`                 | Model used for PII detection |
| `OPENAI_API_BASE` | `https://api.openai.com` | Upstream LLM provider        |

Copy `.env.example` to `.env` and adjust as needed.

---

## 📄 License

MIT License — see [LICENSE](LICENSE) for details.

---

## 👤 Author

**Karcsihack**

- LinkedIn: [Carlos Molina Bou](https://www.linkedin.com/in/carlos-molina-bou-19b09811a/)
- GitHub: [github.com/Karcsihack](https://github.com/Karcsihack)
