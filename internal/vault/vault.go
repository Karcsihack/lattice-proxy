// Package vault provides a request-scoped, thread-safe store that maps
// PII values to opaque anonymization tokens and back.
package vault

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// Vault stores the bidirectional mapping between PII values and tokens.
// Each HTTP request should create its own Vault instance so that tokens
// never leak across sessions.
type Vault struct {
	mu      sync.RWMutex
	data    map[string]string // token  -> real value
	reverse map[string]string // real value -> token (dedup within request)
	seq     atomic.Uint64
}

// New returns an empty, ready-to-use Vault.
func New() *Vault {
	return &Vault{
		data:    make(map[string]string),
		reverse: make(map[string]string),
	}
}

// Tokenize stores a PII value and returns its anonymization token.
// If the same value was already stored in this Vault, the existing token
// is returned (idempotent within a request).
//
// entityType should be one of the type strings produced by the NER engine
// (e.g. "PERSON", "PHONE", "LOCATION").
func (v *Vault) Tokenize(entityType, realValue string) string {
	v.mu.Lock()
	defer v.mu.Unlock()

	if token, ok := v.reverse[realValue]; ok {
		return token
	}

	id := v.seq.Add(1)
	token := fmt.Sprintf("[%s_%d]", entityType, id)
	v.data[token] = realValue
	v.reverse[realValue] = token
	return token
}

// TokenizeWithID stores a PII value using the model-suggested token ID
// (e.g. "USER_1" from the Master System Prompt output).
// Brackets are added to make the token unambiguous in free text: [USER_1].
// If the same realValue is already stored, the existing token is returned.
// If the same suggestedID is already in use for a different value, a new
// numeric suffix is generated to avoid collisions.
func (v *Vault) TokenizeWithID(entityType, realValue, suggestedID string) string {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Dedup: same value already tokenized.
	if token, ok := v.reverse[realValue]; ok {
		return token
	}

	token := "[" + suggestedID + "]"

	// Collision: the suggested token is already used for a different value.
	if _, taken := v.data[token]; taken {
		id := v.seq.Add(1)
		token = fmt.Sprintf("[%s_%d]", entityType, id)
	} else {
		// Advance seq so auto-generated tokens never collide with suggested ones.
		v.seq.Add(1)
	}

	v.data[token] = realValue
	v.reverse[realValue] = token
	return token
}

// Detokenize replaces every known token in text with its original PII value.
func (v *Vault) Detokenize(text string) string {
	v.mu.RLock()
	defer v.mu.RUnlock()

	result := text
	for token, real := range v.data {
		result = strings.ReplaceAll(result, token, real)
	}
	return result
}

// Size returns the number of PII entries stored in the Vault.
func (v *Vault) Size() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.data)
}
