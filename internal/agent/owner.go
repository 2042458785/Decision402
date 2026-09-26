package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

// This is a local, single-user demo vault. Agent secrets never enter the model
// prompt or browser responses; the ignored artifacts directory must be backed up
// if a funded test wallet needs to survive this machine.
type agentRecord struct {
	ID          string `json:"id"`
	Owner       string `json:"owner"`
	Name        string `json:"name"`
	Wallet      string `json:"wallet"`
	ModelURL    string `json:"model_url"`
	ModelName   string `json:"model_name"`
	ModelAPIKey string `json:"model_api_key,omitempty"`
}

type agentView struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Name      string `json:"name"`
	Wallet    string `json:"wallet"`
	ModelURL  string `json:"model_url"`
	ModelName string `json:"model_name"`
}

func (a agentRecord) view() agentView {
	return agentView{a.ID, a.Owner, a.Name, a.Wallet, a.ModelURL, a.ModelName}
}

type challenge struct {
	address string
	message string
	expires time.Time
}
type session struct {
	address string
	expires time.Time
}
type ownerAuth struct {
	mu         sync.Mutex
	challenges map[string]challenge
	sessions   map[string]session
}

func newOwnerAuth() *ownerAuth {
	return &ownerAuth{challenges: map[string]challenge{}, sessions: map[string]session{}}
}
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func (a *App) agentFile(id, ext string) string { return filepath.Join(a.agentDir, id+ext) }

func (a *App) loadAgents() error {
	if err := os.MkdirAll(a.agentDir, 0700); err != nil {
		return err
	}
	info, err := os.Stat(a.agentDir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("Agent vault directory must be private (0700)")
	}
	files, err := filepath.Glob(filepath.Join(a.agentDir, "*.json"))
	if err != nil {
		return err
	}
	a.agents = map[string]agentRecord{}
	for _, path := range files {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("Agent vault metadata must be a regular 0600 file")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var agent agentRecord
		if json.Unmarshal(b, &agent) != nil || !taskIDPattern.MatchString(agent.ID) || filepath.Base(path) != agent.ID+".json" || !addressPattern.MatchString(agent.Owner) || !addressPattern.MatchString(agent.Wallet) || agent.ModelURL != "https://api.deepseek.com" {
			return errors.New("Agent vault metadata is invalid")
		}
		keyInfo, err := os.Lstat(a.agentFile(agent.ID, ".key"))
		if err != nil || !keyInfo.Mode().IsRegular() || keyInfo.Mode().Perm()&0077 != 0 {
			return errors.New("Agent wallet key must be a regular 0600 file")
		}
		key, err := os.ReadFile(a.agentFile(agent.ID, ".key"))
		if err != nil {
			return err
		}
		pk, err := crypto.HexToECDSA(strings.TrimSpace(string(key)))
		if err != nil || !strings.EqualFold(crypto.PubkeyToAddress(pk.PublicKey).Hex(), agent.Wallet) {
			return errors.New("Agent wallet does not match its key")
		}
		a.agents[agent.ID] = agent
	}
	return nil
}

func writePrivate(path string, value []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(value); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err = f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

func (a *App) issueChallenge(w http.ResponseWriter, r *http.Request) {
	address := r.URL.Query().Get("address")
	if !addressPattern.MatchString(address) {
		jsonResponse(w, 400, map[string]string{"error": "Invalid owner address"})
		return
	}
	nonce, err := randomHex(16)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Could not create challenge"})
		return
	}
	message := fmt.Sprintf("Decision402 local sign-in\nOrigin: http://%s\nWallet: %s\nNonce: %s\nNo transaction or token approval.", a.host, address, nonce)
	a.auth.mu.Lock()
	for k, c := range a.auth.challenges {
		if time.Now().After(c.expires) {
			delete(a.auth.challenges, k)
		}
	}
	a.auth.challenges[nonce] = challenge{address: address, message: message, expires: time.Now().Add(5 * time.Minute)}
	a.auth.mu.Unlock()
	jsonResponse(w, 200, map[string]string{"nonce": nonce, "message": message})
}

func (a *App) openSession(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2048))
	var input struct {
		Address   string `json:"address"`
		Nonce     string `json:"nonce"`
		Signature string `json:"signature"`
	}
	if err != nil || strictJSON(b, &input) != nil {
		jsonResponse(w, 400, map[string]string{"error": "Invalid sign-in request"})
		return
	}
	a.auth.mu.Lock()
	c, ok := a.auth.challenges[input.Nonce]
	delete(a.auth.challenges, input.Nonce)
	a.auth.mu.Unlock()
	if !ok || time.Now().After(c.expires) || !strings.EqualFold(c.address, input.Address) {
		jsonResponse(w, 401, map[string]string{"error": "Challenge expired or mismatched"})
		return
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(input.Signature, "0x"))
	if err != nil || len(sig) != 65 {
		jsonResponse(w, 401, map[string]string{"error": "Invalid wallet signature"})
		return
	}
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	if sig[64] > 1 {
		jsonResponse(w, 401, map[string]string{"error": "Invalid wallet signature"})
		return
	}
	hash := crypto.Keccak256([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(c.message), c.message)))
	pub, err := crypto.SigToPub(hash, sig)
	if err != nil || !strings.EqualFold(crypto.PubkeyToAddress(*pub).Hex(), c.address) {
		jsonResponse(w, 401, map[string]string{"error": "Wallet signature did not match"})
		return
	}
	token, err := randomHex(32)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Could not create session"})
		return
	}
	a.auth.mu.Lock()
	a.auth.sessions[token] = session{address: c.address, expires: time.Now().Add(time.Hour)}
	a.auth.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "decision402_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 3600})
	jsonResponse(w, 200, map[string]string{"owner": c.address})
}

func (a *App) sessionOwner(r *http.Request) string {
	c, err := r.Cookie("decision402_session")
	if err != nil {
		return ""
	}
	a.auth.mu.Lock()
	defer a.auth.mu.Unlock()
	s, ok := a.auth.sessions[c.Value]
	if !ok || time.Now().After(s.expires) {
		delete(a.auth.sessions, c.Value)
		return ""
	}
	return s.address
}
func (a *App) requireOwner(w http.ResponseWriter, r *http.Request) string {
	owner := a.sessionOwner(r)
	if owner == "" {
		jsonResponse(w, 401, map[string]string{"error": "Connect and sign in with your owner wallet"})
	}
	return owner
}
func (a *App) createAgent(w http.ResponseWriter, r *http.Request) {
	owner := a.requireOwner(w, r)
	if owner == "" {
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	var input struct {
		Name        string `json:"name"`
		ModelURL    string `json:"model_url"`
		ModelName   string `json:"model_name"`
		ModelAPIKey string `json:"model_api_key"`
	}
	if err != nil || strictJSON(b, &input) != nil {
		jsonResponse(w, 400, map[string]string{"error": "Invalid agent configuration"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if len(input.Name) < 1 || len(input.Name) > 40 || input.ModelURL != "https://api.deepseek.com" || (input.ModelName != "deepseek-flash" && input.ModelName != "deepseek-v4-pro") || len(input.ModelAPIKey) > 256 || strings.ContainsAny(input.ModelAPIKey, "\r\n") {
		jsonResponse(w, 400, map[string]string{"error": "Use a name, the official DeepSeek endpoint, and a supported model"})
		return
	}
	if input.ModelAPIKey == "" && a.model.Key == "" {
		jsonResponse(w, 400, map[string]string{"error": "Enter a DeepSeek API key or configure the server default"})
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.agents) >= 20 {
		jsonResponse(w, 409, map[string]string{"error": "Local demo agent limit reached"})
		return
	}
	id, err := randomHex(16)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Could not generate agent"})
		return
	}
	pk, err := crypto.GenerateKey()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Could not generate wallet"})
		return
	}
	agent := agentRecord{ID: id, Owner: owner, Name: input.Name, Wallet: crypto.PubkeyToAddress(pk.PublicKey).Hex(), ModelURL: input.ModelURL, ModelName: input.ModelName, ModelAPIKey: input.ModelAPIKey}
	keyPath := a.agentFile(id, ".key")
	if err := writePrivate(keyPath, []byte(hex.EncodeToString(crypto.FromECDSA(pk)))); err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Could not save wallet"})
		return
	}
	metadata, _ := json.Marshal(agent)
	if err := writePrivate(a.agentFile(id, ".json"), metadata); err != nil {
		os.Remove(keyPath)
		jsonResponse(w, 500, map[string]string{"error": "Could not save agent"})
		return
	}
	d, err := os.Open(a.agentDir)
	if err != nil || d.Sync() != nil {
		if d != nil {
			d.Close()
		}
		jsonResponse(w, 500, map[string]string{"error": "Agent created on disk but could not be confirmed; inspect vault before retrying"})
		return
	}
	d.Close()
	a.agents[id] = agent
	jsonResponse(w, 201, agent.view())
}
