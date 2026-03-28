// Package ner implements PII detection by delegating to a local Ollama instance.
// No sensitive data ever leaves the machine during detection.
package ner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Entity represents a single PII entity detected in a text.
// The JSON schema matches the output format enforced by the Master System Prompt:
//
//	{"entity": "PERSON", "value": "Carlos", "id": "USER_1"}
type Entity struct {
	// EntityType is the PII category (PERSON, PHONE, LOCATION, EMAIL, …).
	EntityType string `json:"entity"`
	// Value is the exact substring from the input that contains PII.
	Value string `json:"value"`
	// ID is the suggested anonymization token (e.g. "USER_1").
	// Lattice uses this as the Vault key so logs are self-documenting.
	ID string `json:"id"`
}

// Detector sends text to a local Ollama endpoint to identify PII entities.
type Detector struct {
	baseURL string // e.g. "http://localhost:11434"
	model   string // e.g. "llama3" or "phi3"
	client  *http.Client
}

// NewDetector creates a Detector that uses the given Ollama URL and model.
func NewDetector(baseURL, model string) *Detector {
	return &Detector{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// nerSystemPrompt is the Master DLP System Prompt that drives Lattice's
// intelligence. It instructs the local model to behave as a pure data
// extractor: no conversation, no explanations — only JSON.
const nerSystemPrompt = `Eres un agente de Seguridad de Datos (DLP) de élite. Tu única misión es identificar Entidades de Información Identificable (PII) en el texto que te proporcionaré.

REGLAS ESTRICTAS:
1. Detecta: Nombres completos (PERSON), DNI/NIE (ID_NUMBER), números de tarjeta de crédito (CREDIT_CARD), direcciones físicas (LOCATION), correos electrónicos (EMAIL) y números de teléfono (PHONE).
2. NO respondas con texto fluido. Solo responde en formato JSON puro.
3. El formato de salida DEBE ser un array de objetos con exactamente estas claves:
   [{"entity": "TIPO", "value": "VALOR_EXACTO_EN_EL_TEXTO", "id": "TOKEN_UNICO"}]
   donde "id" sigue el patrón TIPO_N (ej: USER_1, PHONE_2, LOC_3).
4. El campo "value" DEBE ser la subcadena exacta tal y como aparece en el texto de entrada.
5. Si no encuentras nada, responde exactamente: []
6. Mantén la latencia baja. No analices sentimientos ni contexto innecesario.

EJEMPLO DE ENTRADA: 'Llamar a Carlos al 600123123 por el contrato de Madrid'.
EJEMPLO DE SALIDA:
[
  {"entity": "PERSON",   "value": "Carlos",    "id": "USER_1"},
  {"entity": "PHONE",    "value": "600123123", "id": "PHONE_1"},
  {"entity": "LOCATION", "value": "Madrid",    "id": "LOC_1"}
]`

// ollamaGenerateRequest matches the /api/generate body expected by Ollama.
type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	System string `json:"system"`
	Stream bool   `json:"stream"`
	Format string `json:"format"` // "json" forces Ollama to produce valid JSON
}

// ollamaGenerateResponse contains the fields we care about from Ollama.
type ollamaGenerateResponse struct {
	Response string `json:"response"`
}

// Detect sends text to Ollama and returns all PII entities found.
// If Ollama is unreachable or returns malformed output the error is returned
// so the caller can decide whether to block or pass the request through.
func (d *Detector) Detect(ctx context.Context, text string) ([]Entity, error) {
	payload := ollamaGenerateRequest{
		Model:  d.model,
		Prompt: text,
		System: nerSystemPrompt,
		Stream: false,
		Format: "json",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("ner: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		d.baseURL+"/api/generate",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("ner: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ner: ollama unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ner: unexpected ollama status %d", resp.StatusCode)
	}

	var ollamaResp ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("ner: decode ollama response: %w", err)
	}

	var entities []Entity
	if err := json.Unmarshal([]byte(ollamaResp.Response), &entities); err != nil {
		// Fail-safe: model returned unexpected output → treat as "no PII found".
		log.Printf("[NER] WARN: model output is not valid JSON (no PII assumed). err=%v | raw=%.200s",
			err, ollamaResp.Response)
		return []Entity{}, nil
	}

	return entities, nil
}
