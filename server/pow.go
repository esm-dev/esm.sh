package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Generic proof-of-work (hashcash-style SHA-256 prefix) challenges, used to
// gate expensive or abusable endpoints. A feature registers a scope in
// `powPolicies` and then calls `powVerify(scope, id, nonce)` to check the
// solution submitted by the client.
//
// Challenges are stateless: the id carries the salt and the expiry, signed
// with a per-process key, so minting them cannot exhaust server memory. Only
// solved challenges are remembered, until they expire, to keep them single-use.

const powSolvedMaxSize = 10000 // in-memory cap of solved, unexpired challenges

var errUnknownPowScope = errors.New("unknown proof-of-work scope")

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

var powKey = []byte(randomHex(32))

var powSolved = struct {
	sync.Mutex
	m map[string]int64 // challenge id -> expiry (unix seconds)
}{m: make(map[string]int64)}

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

// powSign signs the `salt.expiry` payload of a challenge for the given scope.
func powSign(scope string, payload string) string {
	mac := hmac.New(sha256.New, powKey)
	mac.Write([]byte(scope + "|" + payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// newPowChallenge mints a one-time challenge for the given scope.
func newPowChallenge(scope string) (*powChallengeResponse, error) {
	policy, ok := powPolicies[scope]
	if !ok {
		return nil, errUnknownPowScope
	}
	salt := randomHex(16)
	payload := salt + "." + strconv.FormatInt(time.Now().Add(policy.ttl).Unix(), 10)
	return &powChallengeResponse{ID: payload + "." + powSign(scope, payload), Salt: salt, Scope: scope, Difficulty: policy.difficulty}, nil
}

// powVerify checks a solved challenge, then uses it up when allow passes. A
// challenge is single-use and only valid for the scope it was minted for, so a
// valid solution cannot be replayed across endpoints.
// [warning] the solution is reserved before allow runs, so parallel replays
// call allow once; it is forgotten when allow fails, so the requests allow
// rejects cannot fill the solved store and lock out every client.
func powVerify(scope string, id string, nonce string, allow func() bool) bool {
	policy, ok := powPolicies[scope]
	if !ok {
		return false
	}
	i := strings.LastIndexByte(id, '.')
	if i < 0 || !hmac.Equal([]byte(id[i+1:]), []byte(powSign(scope, id[:i]))) {
		return false
	}
	salt, expStr, _ := strings.Cut(id[:i], ".")
	exp, err := strconv.ParseInt(expStr, 10, 64)
	now := time.Now().Unix()
	if err != nil || now > exp {
		return false
	}
	sum := sha256.Sum256([]byte(salt + nonce))
	if !strings.HasPrefix(hex.EncodeToString(sum[:]), strings.Repeat("0", policy.difficulty)) {
		return false
	}
	powSolved.Lock()
	if _, used := powSolved.m[id]; used {
		powSolved.Unlock()
		return false
	}
	if len(powSolved.m) >= powSolvedMaxSize {
		for k, e := range powSolved.m {
			if now > e {
				delete(powSolved.m, k)
			}
		}
		// fail closed: forgetting an unexpired solution would allow its replay
		if len(powSolved.m) >= powSolvedMaxSize {
			powSolved.Unlock()
			return false
		}
	}
	powSolved.m[id] = exp
	powSolved.Unlock()
	if allow != nil && !allow() {
		powSolved.Lock()
		delete(powSolved.m, id)
		powSolved.Unlock()
		return false
	}
	return true
}
