package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

// Generic proof-of-work (hashcash-style SHA-256 prefix) challenges, used to
// gate expensive or abusable endpoints. A feature registers a scope in
// `powPolicies` and then calls `powVerify(scope, id, nonce)` to check the
// solution submitted by the client.

const powChallengeMaxSize = 10000 // in-memory cap of pending challenges

var (
	errUnknownPowScope = errors.New("unknown proof-of-work scope")
	errPowStoreFull    = errors.New("too many pending proof-of-work challenges")
)

// powPolicy describes how hard and how long the challenges of a scope are.
type powPolicy struct {
	difficulty int // leading zero hex chars required
	ttl        time.Duration
}

// powPolicies maps a proof-of-work scope to its policy. New endpoints that
// need bot protection register a scope here instead of growing their own
// challenge store.
var powPolicies = map[string]powPolicy{
	"purge": {difficulty: 4, ttl: 2 * time.Minute},
}

type powChallenge struct {
	salt       string
	scope      string
	difficulty int
	expiresAt  time.Time
}

var powChallengeStore = struct {
	sync.Mutex
	m map[string]powChallenge
}{m: make(map[string]powChallenge)}

// powChallengeResponse is the JSON body served by `GET /pow/challenge`.
type powChallengeResponse struct {
	ID         string `json:"id"`
	Salt       string `json:"salt"`
	Scope      string `json:"scope"`
	Difficulty int    `json:"difficulty"`
}

// randomHex returns cryptographically-secure random bytes encoded as hex.
func randomHex(size int) string {
	buffer := make([]byte, size)
	rand.Read(buffer) // crypto/rand.Read never fails since Go 1.24
	return hex.EncodeToString(buffer)
}

// newPowChallenge mints a one-time challenge for the given scope. It fails
// when the scope is unknown or too many challenges are pending (expired ones
// are garbage collected once the store runs full).
func newPowChallenge(scope string) (*powChallengeResponse, error) {
	policy, ok := powPolicies[scope]
	if !ok {
		return nil, errUnknownPowScope
	}
	now := time.Now()
	powChallengeStore.Lock()
	defer powChallengeStore.Unlock()
	if len(powChallengeStore.m) >= powChallengeMaxSize {
		for id, stored := range powChallengeStore.m {
			if now.After(stored.expiresAt) {
				delete(powChallengeStore.m, id)
			}
		}
		if len(powChallengeStore.m) >= powChallengeMaxSize {
			return nil, errPowStoreFull
		}
	}
	challenge := powChallenge{salt: randomHex(16), scope: scope, difficulty: policy.difficulty, expiresAt: now.Add(policy.ttl)}
	id := randomHex(16)
	powChallengeStore.m[id] = challenge
	return &powChallengeResponse{ID: id, Salt: challenge.salt, Scope: scope, Difficulty: challenge.difficulty}, nil
}

// powVerify checks a solved challenge. A challenge is single-use and only
// valid for the scope it was minted for, so a valid solution cannot be
// replayed across endpoints.
func powVerify(scope string, id string, nonce string) bool {
	powChallengeStore.Lock()
	challenge, ok := powChallengeStore.m[id]
	delete(powChallengeStore.m, id)
	powChallengeStore.Unlock()
	if !ok || challenge.scope != scope || time.Now().After(challenge.expiresAt) {
		return false
	}
	sum := sha256.Sum256([]byte(challenge.salt + nonce))
	return strings.HasPrefix(hex.EncodeToString(sum[:]), strings.Repeat("0", challenge.difficulty))
}
