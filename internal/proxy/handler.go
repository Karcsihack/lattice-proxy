// Package proxy implements the reverse-proxy logic for Lattice.
//
// Request lifecycle:
//  1. INBOUND  – receive OpenAI-compatible JSON, extract messages, detect PII
//               via NER (each message processed concurrently with goroutines),
//               replace PII with opaque tokens stored in a per-request Vault.
//  2. FORWARD  – send the anonymized request to the upstream API (OpenAI or
//               any compatible provider).
//  3. OUTBOUND – read the upstream response, run Vault.Detokenize to restore
//               the original values, return the clean response to the caller.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"lattice-proxy/internal/ner"
	"lattice-proxy/internal/vault"
)

// ChatMessage mirrors the OpenAI chat message schema.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Handler is the HTTP handler for the /v1/chat/completions endpoint.
type Handler struct {
	detector   *ner.Detector
	openAIBase string // e.g. "https://api.openai.com"
	client     *http.Client
}

// NewHandler creates a Handler configured with the given NER detector and
// upstream base URL.
func NewHandler(detector *ner.Detector, openAIBase string) *Handler {
	return &Handler{
		detector:   detector,
		openAIBase: openAIBase,
		client:     &http.Client{Timeout: 90 * time.Second},
	}
}

// ChatCompletions handles POST /v1/chat/completions.
//
// It anonymizes the request, forwards it to the upstream API, then
// de-anonymizes the response — all in a single request context.
func (h *Handler) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, `{"error":{"message":"method not allowed"}}`)
		return
	}

	start := time.Now()
	ctx := r.Context()

	// ── 1. Read body (4 MB guard against oversized payloads) ─────────────────
	rawBody, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, `{"error":{"message":"failed to read request body"}}`)
		return
	}

	// Parse as a generic JSON map so we transparently pass through any fields
	// we do not explicitly handle (temperature, top_p, tools, …).
	var reqMap map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &reqMap); err != nil {
		writeJSON(w, http.StatusBadRequest, `{"error":{"message":"invalid JSON"}}`)
		return
	}

	// Extract and validate the messages array.
	var messages []ChatMessage
	if raw, ok := reqMap["messages"]; ok {
		if err := json.Unmarshal(raw, &messages); err != nil {
			writeJSON(w, http.StatusBadRequest, `{"error":{"message":"invalid messages field"}}`)
			return
		}
	}

	// ── 2. INBOUND: Anonymize ─────────────────────────────────────────────────
	requestVault := vault.New()
	anonMessages, origTexts := h.anonymizeMessages(ctx, messages, requestVault)
	anonLatency := time.Since(start)

	// Structured console log: original vs. anonymized, per message.
	for i, orig := range origTexts {
		if i < len(anonMessages) && orig != anonMessages[i].Content {
			log.Printf("[LATTICE] ┌ Texto Original  : %s", truncate(orig, 300))
			log.Printf("[LATTICE] └ Texto a la Nube : %s", truncate(anonMessages[i].Content, 300))
		}
	}

	// Re-inject anonymized messages and force stream=false so we can inspect
	// the full response body for de-anonymization.
	anonRaw, _ := json.Marshal(anonMessages)
	reqMap["messages"] = anonRaw
	reqMap["stream"] = json.RawMessage(`false`)

	newBody, err := json.Marshal(reqMap)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, `{"error":{"message":"internal serialization error"}}`)
		return
	}

	// ── 3. FORWARD to upstream ────────────────────────────────────────────────
	upstreamResp, err := h.forward(ctx, r, newBody)
	if err != nil {
		log.Printf("[LATTICE] ERROR upstream: %v", err)
		writeJSON(w, http.StatusBadGateway, `{"error":{"message":"upstream API unavailable"}}`)
		return
	}
	defer upstreamResp.Body.Close()

	upstreamBody, err := io.ReadAll(upstreamResp.Body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, `{"error":{"message":"failed to read upstream response"}}`)
		return
	}

	// ── 4. OUTBOUND: De-anonymize ─────────────────────────────────────────────
	cleanBody := requestVault.Detokenize(string(upstreamBody))

	totalLatency := time.Since(start)
	log.Printf("[LATTICE] ✓ Completado | entidades=%d | latencia_anon=%dms | latencia_total=%dms",
		requestVault.Size(), anonLatency.Milliseconds(), totalLatency.Milliseconds())

	// Copy upstream response headers verbatim (e.g. Content-Type, x-request-id).
	for k, vals := range upstreamResp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(upstreamResp.StatusCode)
	fmt.Fprint(w, cleanBody)
}

// anonymizeMessages runs NER on every non-system message concurrently using
// one goroutine per message, then replaces all detected PII with vault tokens.
// It returns the anonymized messages in the original order, and the original
// message contents for logging.
func (h *Handler) anonymizeMessages(
	ctx context.Context,
	messages []ChatMessage,
	v *vault.Vault,
) ([]ChatMessage, []string) {
	type result struct {
		idx     int
		message ChatMessage
	}

	origTexts := make([]string, len(messages))
	ch := make(chan result, len(messages))

	var wg sync.WaitGroup
	for i, msg := range messages {
		origTexts[i] = msg.Content
		wg.Add(1)

		go func(idx int, m ChatMessage) {
			defer wg.Done()

			// System prompts come from the developer, not the end-user; skip them
			// to avoid altering prompt instructions.
			if m.Role == "system" || strings.TrimSpace(m.Content) == "" {
				ch <- result{idx, m}
				return
			}

			entities, err := h.detector.Detect(ctx, m.Content)
			if err != nil {
				// Fail-open: if NER is unavailable, pass the message as-is.
				// Operators can make this fail-closed at the business-logic layer.
				log.Printf("[LATTICE] WARN NER falló (mensaje %d sin anonimizar): %v", idx, err)
				ch <- result{idx, m}
				return
			}

			content := m.Content
			for _, entity := range entities {
				if entity.Value == "" {
					continue
				}
				// Use the model-suggested ID as the vault token when available;
				// fall back to generating one from the entity type.
				var token string
				if entity.ID != "" {
					token = v.TokenizeWithID(entity.EntityType, entity.Value, entity.ID)
				} else {
					token = v.Tokenize(entity.EntityType, entity.Value)
				}
				content = strings.ReplaceAll(content, entity.Value, token)
			}

			ch <- result{idx, ChatMessage{Role: m.Role, Content: content}}
		}(i, msg)
	}

	// Drain the channel once all goroutines are done.
	go func() {
		wg.Wait()
		close(ch)
	}()

	anon := make([]ChatMessage, len(messages))
	for r := range ch {
		anon[r.idx] = r.message
	}
	return anon, origTexts
}

// forward sends body to the upstream OpenAI-compatible API, forwarding only
// the Authorization header from the original client request.
func (h *Handler) forward(ctx context.Context, original *http.Request, body []byte) (*http.Response, error) {
	url := h.openAIBase + "/v1/chat/completions"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Forward the client's API key to the upstream provider.
	// We do NOT log this value.
	if auth := original.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}

	return h.client.Do(req)
}

// writeJSON writes a pre-serialized JSON string with the given HTTP status.
func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprint(w, body)
}

// truncate shortens s to at most n runes, appending "…" if cut.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
