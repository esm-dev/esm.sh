package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ije/gox/log"
)

// `POST /purge` can be gated behind GitHub OAuth instead of the proof-of-work
// challenge. When `githubClientId`/`githubClientSecret` are configured the
// caller must first sign in with GitHub; the signed session cookie identifies
// the purging account (so abuse can be attributed and rate-limited per user).
// Without those settings the proof-of-work flow stays in charge.
const (
	purgeSessionCookie    = "purge_session"
	purgeOAuthStateCookie = "purge_oauth_state"
	purgeSessionTTL       = 24 * time.Hour
	purgeOAuthStateTTL    = 10 * time.Minute
	purgeOAuthScope       = "read:user"
)

// Overridable endpoints, so tests can point the flow at a local server.
var (
	githubOAuthAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubOAuthTokenURL     = "https://github.com/login/oauth/access_token"
	githubUserAPIURL        = "https://api.github.com/user"
)

// purgeSession is the signed cookie payload of a signed-in purge user.
type purgeSession struct {
	Login     string `json:"login"`
	ExpiresAt int64  `json:"exp"`
}

// purgeOAuthEnabled reports whether `POST /purge` is gated by GitHub OAuth.
func purgeOAuthEnabled() bool {
	return config.PurgeCache && config.GithubClientID != "" && config.GithubClientSecret != ""
}

func purgeSessionKey() []byte {
	sum := sha256.Sum256([]byte("esm.sh/purge-session/" + config.GithubClientSecret))
	return sum[:]
}

// signPurgeSession encodes a session as `<base64url(json)>.<hex hmac>`.
func signPurgeSession(session *purgeSession) string {
	payload, _ := json.Marshal(session)
	b64 := btoaUrl(string(payload))
	mac := hmac.New(sha256.New, purgeSessionKey())
	mac.Write([]byte(b64))
	return b64 + "." + hex.EncodeToString(mac.Sum(nil))
}

// parsePurgeSession verifies the signature and expiry of a session cookie.
func parsePurgeSession(value string) *purgeSession {
	i := strings.LastIndexByte(value, '.')
	if i <= 0 {
		return nil
	}
	b64, sig := value[:i], value[i+1:]
	mac := hmac.New(sha256.New, purgeSessionKey())
	mac.Write([]byte(b64))
	if !hmac.Equal([]byte(sig), []byte(hex.EncodeToString(mac.Sum(nil)))) {
		return nil
	}
	raw, err := atobUrl(b64)
	if err != nil {
		return nil
	}
	var session purgeSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil
	}
	if session.Login == "" || time.Now().Unix() > session.ExpiresAt {
		return nil
	}
	return &session
}

// purgeSessionFromRequest returns the signed-in user of the request, if any.
func purgeSessionFromRequest(r *http.Request) *purgeSession {
	cookie, err := r.Cookie(purgeSessionCookie)
	if err != nil {
		return nil
	}
	return parsePurgeSession(cookie.Value)
}

func setPurgeCookie(w http.ResponseWriter, r *http.Request, name string, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/purge",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   strings.HasPrefix(getOrigin(r), "https://"),
	})
}

func purgeRedirect(w http.ResponseWriter, location string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusFound)
}

// purgeOAuthLogin redirects the browser to GitHub to start the OAuth flow.
func purgeOAuthLogin(w http.ResponseWriter, r *http.Request) {
	if !purgeOAuthEnabled() {
		writeStatus(w, 404, "GitHub OAuth is not configured")
		return
	}
	state := randomHex(16)
	setPurgeCookie(w, r, purgeOAuthStateCookie, state, int(purgeOAuthStateTTL.Seconds()))
	query := url.Values{
		"client_id":    {config.GithubClientID},
		"redirect_uri": {getOrigin(r) + "/purge/callback"},
		"scope":        {purgeOAuthScope},
		"state":        {state},
	}
	purgeRedirect(w, githubOAuthAuthorizeURL+"?"+query.Encode())
}

// purgeOAuthCallback completes the flow: exchange the code for a token, read
// the GitHub login and mint a signed session cookie.
func purgeOAuthCallback(w http.ResponseWriter, r *http.Request, logger *log.Logger) {
	if !purgeOAuthEnabled() {
		writeStatus(w, 404, "GitHub OAuth is not configured")
		return
	}
	fail := func(message string) {
		purgeRedirect(w, "/purge?error="+url.QueryEscape(message))
	}
	stateCookie, err := r.Cookie(purgeOAuthStateCookie)
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		fail("invalid oauth state")
		return
	}
	setPurgeCookie(w, r, purgeOAuthStateCookie, "", -1)
	code := r.URL.Query().Get("code")
	if code == "" {
		fail("missing oauth code")
		return
	}
	login, err := githubUserLogin(code, getOrigin(r)+"/purge/callback")
	if err != nil {
		logger.Errorf("purge oauth callback: %v", err)
		fail("failed to sign in with GitHub")
		return
	}
	session := &purgeSession{Login: login, ExpiresAt: time.Now().Add(purgeSessionTTL).Unix()}
	setPurgeCookie(w, r, purgeSessionCookie, signPurgeSession(session), int(purgeSessionTTL.Seconds()))
	purgeRedirect(w, "/purge")
}

// purgeOAuthLogout clears the session cookie.
func purgeOAuthLogout(w http.ResponseWriter, r *http.Request) {
	setPurgeCookie(w, r, purgeSessionCookie, "", -1)
	purgeRedirect(w, "/purge")
}

// githubUserLogin exchanges an OAuth code for an access token and resolves the
// authenticated user's login name.
func githubUserLogin(code string, redirectURI string) (string, error) {
	form := url.Values{
		"client_id":     {config.GithubClientID},
		"client_secret": {config.GithubClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}
	req, err := http.NewRequest(http.MethodPost, githubOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := purgeHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, MB))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %s", res.Status)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return "", err
	}
	if token.AccessToken == "" {
		if token.Error != "" {
			return "", errors.New(token.Error)
		}
		return "", errors.New("empty access token")
	}

	req, err = http.NewRequest(http.MethodGet, githubUserAPIURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	res, err = purgeHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err = io.ReadAll(io.LimitReader(res.Body, MB))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("user endpoint returned %s", res.Status)
	}
	var user struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		return "", err
	}
	if user.Login == "" {
		return "", errors.New("empty login")
	}
	return user.Login, nil
}
