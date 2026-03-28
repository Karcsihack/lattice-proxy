// Lattice — Privacy Reverse Proxy for LLMs.
//
// Listens on :8080 and exposes an OpenAI-compatible endpoint.
// All PII is detected locally via Ollama before any data reaches the cloud.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lattice-proxy/internal/ner"
	"lattice-proxy/internal/proxy"
)

func main() {
	ollamaURL   := getEnv("OLLAMA_URL",        "http://localhost:11434")
	ollamaModel := getEnv("OLLAMA_MODEL",       "llama3")
	openAIBase  := getEnv("OPENAI_API_BASE",    "https://api.openai.com")
	listenAddr  := getEnv("LISTEN_ADDR",        ":8080")

	detector := ner.NewDetector(ollamaURL, ollamaModel)
	handler  := proxy.NewHandler(detector, openAIBase)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", handler.ChatCompletions)
	mux.HandleFunc("/health", healthCheck)

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("[LATTICE] Proxy de Privacidad iniciado en %s", listenAddr)
	log.Printf("[LATTICE] NER backend : %s  (modelo: %s)", ollamaURL, ollamaModel)
	log.Printf("[LATTICE] Upstream API: %s", openAIBase)

	// Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[LATTICE] Error fatal: %v", err)
		}
	}()

	<-quit
	log.Println("[LATTICE] Apagando servidor...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("[LATTICE] Shutdown forzado: %v", err)
	}
	log.Println("[LATTICE] Servidor detenido correctamente.")
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"lattice-proxy"}`))
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
