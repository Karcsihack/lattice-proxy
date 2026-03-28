// mock_ollama.go — Servidor que imita la API de Ollama para testing local.
// Detecta PII usando Regex en lugar de un LLM, permitiendo probar Lattice
// sin tener Ollama ni ningún modelo descargado.
//
// Uso:
//
//	go run mock_ollama.go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
)

type generateRequest struct {
	Prompt string `json:"prompt"`
}

type generateResponse struct {
	Response string `json:"response"`
}

type entity struct {
	EntityType string `json:"entity"`
	Value      string `json:"value"`
	ID         string `json:"id"`
}

// piiPattern agrupa los detectores por tipo.
var piiPatterns = []struct {
	entityType string
	prefix     string
	re         *regexp.Regexp
}{
	{"EMAIL", "EMAIL", regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)},
	{"PHONE", "PHONE", regexp.MustCompile(`(?:\+34\s?)?[6789]\d{2}[\s\-]?\d{3}[\s\-]?\d{3}`)},
	{"ID_NUMBER", "ID", regexp.MustCompile(`\b\d{8}[A-HJ-NP-TV-Z]\b`)},
	{"CREDIT_CARD", "CARD", regexp.MustCompile(`\b(?:\d{4}[\s\-]?){3}\d{4}\b`)},
	// Nombres: palabras capitalizadas que aparecen tras "soy", "llamo", "nombre"
	{"PERSON", "USER", regexp.MustCompile(`(?i)(?:soy|llamo|nombre[:\s]+|llámame\s+)([A-ZÁÉÍÓÚÑ][a-záéíóúñ]+(?:\s[A-ZÁÉÍÓÚÑ][a-záéíóúñ]+)*)`)},
}

func detect(text string) []entity {
	var entities []entity
	seen := map[string]bool{}
	counters := map[string]int{}

	for _, p := range piiPatterns {
		matches := p.re.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			val := m[0]
			// Para PERSON el valor está en el grupo capturado
			if len(m) > 1 && m[1] != "" {
				val = m[1]
			}
			val = strings.TrimSpace(val)
			if val == "" || seen[val] {
				continue
			}
			seen[val] = true
			counters[p.prefix]++
			entities = append(entities, entity{
				EntityType: p.entityType,
				Value:      val,
				ID:         fmt.Sprintf("%s_%d", p.prefix, counters[p.prefix]),
			})
		}
	}
	return entities
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	entities := detect(req.Prompt)
	jsonBytes, _ := json.Marshal(entities)

	resp := generateResponse{Response: string(jsonBytes)}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func main() {
	http.HandleFunc("/api/generate", handleGenerate)
	log.Println("[MOCK-OLLAMA] Escuchando en :11434 — NER via Regex (sin modelo LLM)")
	if err := http.ListenAndServe(":11434", nil); err != nil {
		log.Fatal(err)
	}
}
