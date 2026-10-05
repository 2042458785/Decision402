package agent

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

type agentInput struct {
	WalletKind  string          `json:"wallet_kind,omitempty"`
	Name        string          `json:"name"`
	ModelURL    string          `json:"model_url"`
	ModelName   string          `json:"model_name"`
	ModelAPIKey string          `json:"model_api_key"`
	Password    string          `json:"password"`
	KeyStore    json.RawMessage `json:"keystore,omitempty"`
}

func readSecretRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 24576))
	defer wipe(b)
	if err != nil || strictJSON(b, target) != nil {
		jsonResponse(w, 400, map[string]string{"error": "Invalid wallet request"})
		return false
	}
	return true
}
func validNewPassword(p string) bool {
	return len(p) >= 12 && len(p) <= 1024 && strings.TrimSpace(p) != ""
}
func sessionToken(r *http.Request) string {
	c, e := r.Cookie("decision402_session")
	if e != nil {
		return ""
	}
	return c.Value
}
func (a *App) grantFor(id string, r *http.Request) *walletGrant {
	a.vaultMu.Lock()
	g := a.grants[id]
	a.vaultMu.Unlock()
	if !g.valid(sessionToken(r)) {
		return nil
	}
	return g
}
func (a *App) needsMigration(record agentRecord) bool {
	if record.Version != 1 {
		return true
	}
	_, err := os.Lstat(a.agentFile(record.ID, ".key"))
	return !os.IsNotExist(err)
}
func (a *App) walletView(record agentRecord, r *http.Request) agentView {
	v := record.view()
	v.NeedsMigration = a.needsMigration(record)
	if g := a.grantFor(record.ID, r); g != nil && !v.NeedsMigration {
		v.Locked = false
		v.UnlockUntil = g.expires.Format(time.RFC3339)
	}
	return v
}
func (a *App) ownedAgent(w http.ResponseWriter, r *http.Request) (agentRecord, bool) {
	owner := a.requireOwner(w, r)
	if owner == "" {
		return agentRecord{}, false
	}
	a.mu.Lock()
	record, ok := a.agents[r.PathValue("id")]
	a.mu.Unlock()
	if !ok || !strings.EqualFold(owner, record.Owner) {
		jsonResponse(w, 404, map[string]string{"error": "Agent not found"})
		return agentRecord{}, false
	}
	return record, true
}
func (a *App) lockWallet(id string) {
	a.vaultMu.Lock()
	defer a.vaultMu.Unlock()
	a.lockEpoch[id]++
	a.grants[id].lock()
	delete(a.grants, id)
}
func (a *App) lockOwner(owner string) {
	a.mu.Lock()
	ids := []string{}
	for id, r := range a.agents {
		if strings.EqualFold(r.Owner, owner) {
			ids = append(ids, id)
		}
	}
	a.mu.Unlock()
	for _, id := range ids {
		a.lockWallet(id)
	}
}
func (a *App) closeSession(w http.ResponseWriter, r *http.Request) {
	owner := a.requireOwner(w, r)
	if owner == "" {
		return
	}
	a.lockOwner(owner)
	a.auth.mu.Lock()
	delete(a.auth.sessions, sessionToken(r))
	a.auth.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "decision402_session", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	jsonResponse(w, 200, map[string]bool{"locked": true})
}
func (a *App) createAgent(w http.ResponseWriter, r *http.Request) {
	a.createOrRestoreAgent(w, r, false)
}
func (a *App) restoreAgent(w http.ResponseWriter, r *http.Request) {
	a.createOrRestoreAgent(w, r, true)
}
func (a *App) createOrRestoreAgent(w http.ResponseWriter, r *http.Request, restore bool) {
	owner := a.requireOwner(w, r)
	if owner == "" {
		return
	}
	var input agentInput
	if !readSecretRequest(w, r, &input) {
		return
	}
	defer func() { input.Password = ""; input.ModelAPIKey = "" }()
	if input.WalletKind == "" && !restore {
		input.WalletKind = "smart"
	}
	if input.WalletKind != "" && input.WalletKind != "legacy" && input.WalletKind != "smart" {
		jsonResponse(w, 400, map[string]string{"error": "Unknown wallet kind"})
		return
	}
	if input.WalletKind == "smart" && a.smartChain == nil {
		jsonResponse(w, 503, map[string]string{"error": "Smart wallet RPC is not configured"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if len(input.Name) < 1 || len(input.Name) > 40 || input.ModelURL != "https://api.deepseek.com" || (input.ModelName != "deepseek-flash" && input.ModelName != "deepseek-v4-pro") || len(input.ModelAPIKey) > 256 || strings.ContainsAny(input.ModelAPIKey, "\r\n") {
		jsonResponse(w, 400, map[string]string{"error": "Use a name, the official DeepSeek endpoint, and a supported model"})
		return
	}
	if !validNewPassword(input.Password) {
		jsonResponse(w, 400, map[string]string{"error": "Use a wallet password of 12 to 1024 bytes"})
		return
	}
	if input.ModelAPIKey == "" && a.model.Key == "" {
		jsonResponse(w, 400, map[string]string{"error": "Enter a DeepSeek API key or configure the server default"})
		return
	}
	a.walletOps.Lock()
	defer a.walletOps.Unlock() // Bound the memory consumed by password KDFs.
	a.mu.Lock()
	full := len(a.agents) >= 20
	a.mu.Unlock()
	if full {
		jsonResponse(w, 409, map[string]string{"error": "Local agent limit reached"})
		return
	}
	var pk *ecdsa.PrivateKey
	var err error
	if restore {
		key, e := decryptWallet(input.KeyStore, input.Password)
		if e != nil {
			jsonResponse(w, 400, map[string]string{"error": errWalletPassword.Error()})
			return
		}
		pk = key.PrivateKey
	} else {
		if len(input.KeyStore) != 0 {
			jsonResponse(w, 400, map[string]string{"error": "Use Restore for a wallet backup"})
			return
		}
		pk, err = crypto.GenerateKey()
		if err != nil {
			jsonResponse(w, 500, map[string]string{"error": "Could not generate wallet"})
			return
		}
	}
	defer wipeKey(pk)
	address := crypto.PubkeyToAddress(pk.PublicKey).Hex()
	if input.WalletKind == "smart" && strings.EqualFold(address, owner) {
		jsonResponse(w, 400, map[string]string{"error": "The owner key cannot be used as a session key"})
		return
	}
	a.mu.Lock()
	duplicate := false
	for _, record := range a.agents {
		if strings.EqualFold(record.signingAddress(), address) {
			duplicate = true
		}
	}
	a.mu.Unlock()
	if duplicate {
		jsonResponse(w, 409, map[string]string{"error": "This wallet is already registered"})
		return
	}
	id, err := randomHex(16)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Could not create agent"})
		return
	}
	record := agentRecord{ID: id, Owner: owner, Name: input.Name, Wallet: address, ModelURL: input.ModelURL, ModelName: input.ModelName}
	if input.WalletKind == "smart" {
		record.WalletKind = "smart"
		record.SessionAddress = address
		record.Wallet = ""
	}
	secret := []byte(input.ModelAPIKey)
	defer wipe(secret)
	record, err = encryptRecord(record, pk, secret, input.Password, a.scryptN, a.scryptP)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Could not encrypt agent"})
		return
	}
	data, _ := json.Marshal(record)
	if writePrivate(a.agentFile(id, ".json"), data) != nil {
		jsonResponse(w, 500, map[string]string{"error": "Could not save encrypted agent"})
		return
	}
	if syncDirectory(a.agentDir) != nil {
		jsonResponse(w, 500, map[string]string{"error": "Wallet saved but disk confirmation failed; restart before retrying"})
		return
	}
	a.mu.Lock()
	a.agents[id] = record
	a.mu.Unlock()
	jsonResponse(w, 201, record.view())
}
func (a *App) walletAction(w http.ResponseWriter, r *http.Request) {
	// Serialize changes, then re-read the record so a concurrent migration cannot
	// restore stale plaintext metadata. Lock itself never waits on password KDFs.
	action := r.PathValue("action")
	record, ok := a.ownedAgent(w, r)
	if !ok {
		return
	}
	if action == "lock" {
		a.lockWallet(record.ID)
		jsonResponse(w, 200, a.walletView(record, r))
		return
	}
	a.vaultMu.Lock()
	epoch := a.lockEpoch[record.ID]
	a.vaultMu.Unlock()
	var input struct {
		Password string `json:"password"`
	}
	if !readSecretRequest(w, r, &input) {
		return
	}
	defer func() { input.Password = "" }()
	if len(input.Password) < 1 || len(input.Password) > 1024 {
		jsonResponse(w, 400, map[string]string{"error": "Enter your wallet password"})
		return
	}
	a.walletOps.Lock()
	defer a.walletOps.Unlock()
	record, ok = a.ownedAgent(w, r)
	if !ok {
		return
	}
	if action == "migrate" {
		if !validNewPassword(input.Password) {
			jsonResponse(w, 400, map[string]string{"error": "Use a wallet password of 12 to 1024 bytes"})
			return
		}
		if err := a.migrateWallet(record, input.Password); err != nil {
			jsonResponse(w, 400, map[string]string{"error": err.Error()})
			return
		}
		a.mu.Lock()
		record = a.agents[record.ID]
		a.mu.Unlock()
		jsonResponse(w, 200, a.walletView(record, r))
		return
	}
	if action != "unlock" && action != "backup" {
		jsonResponse(w, 404, map[string]string{"error": "Unknown wallet action"})
		return
	}
	if a.needsMigration(record) {
		jsonResponse(w, 409, map[string]string{"error": "Encrypt this legacy wallet before using it"})
		return
	}
	key, secret, err := decryptRecord(record, input.Password)
	if err != nil {
		jsonResponse(w, 400, map[string]string{"error": errWalletPassword.Error()})
		return
	}
	defer wipeKey(key.PrivateKey)
	defer wipe(secret)
	if action == "backup" {
		w.Header().Set("Content-Disposition", `attachment; filename="decision402-`+record.ID+`.keystore.json"`)
		w.Header().Set("Content-Type", "application/json")
		w.Write(record.KeyStore)
		return
	}
	token := sessionToken(r)
	a.auth.mu.Lock()
	session, exists := a.auth.sessions[token]
	a.auth.mu.Unlock()
	if !exists || !time.Now().Before(session.expires) {
		jsonResponse(w, 401, map[string]string{"error": "Sign in again"})
		return
	}
	expires := time.Now().Add(walletUnlockTTL)
	if session.expires.Before(expires) {
		expires = session.expires
	}
	grant, err := newGrant(key.PrivateKey, secret, token, expires)
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Could not unlock wallet"})
		return
	}
	a.vaultMu.Lock()
	if a.lockEpoch[record.ID] != epoch {
		a.vaultMu.Unlock()
		grant.lock()
		jsonResponse(w, 409, map[string]string{"error": "Wallet was locked during unlock; try again"})
		return
	}
	a.grants[record.ID].lock()
	a.grants[record.ID] = grant
	a.vaultMu.Unlock()
	jsonResponse(w, 200, a.walletView(record, r))
}
func (a *App) migrateWallet(record agentRecord, password string) error {
	a.lockWallet(record.ID)
	// Recover safely even if the previous atomic rename succeeded but its
	// directory sync failed. Never overwrite encrypted metadata with stale data.
	current, err := readPrivate(a.agentFile(record.ID, ".json"))
	if err != nil {
		return errors.New("Could not read wallet metadata")
	}
	defer wipe(current)
	var disk agentRecord
	if json.Unmarshal(current, &disk) != nil || disk.ID != record.ID || disk.Owner != record.Owner || disk.Wallet != record.Wallet || (disk.Version != 0 && disk.Version != 1) {
		return errors.New("Wallet metadata changed; restart before retrying")
	}
	record = disk
	if record.Version == 0 {
		data, err := readPrivate(a.agentFile(record.ID, ".json"))
		if err != nil {
			return errors.New("Could not read legacy agent")
		}
		defer wipe(data)
		var legacy struct {
			ModelAPIKey string `json:"model_api_key"`
		}
		if json.Unmarshal(data, &legacy) != nil {
			return errors.New("Invalid legacy agent")
		}
		defer func() { legacy.ModelAPIKey = "" }()
		raw, err := readPrivate(a.agentFile(record.ID, ".key"))
		if err != nil {
			return errors.New("Could not read legacy wallet")
		}
		defer wipe(raw)
		pk, err := crypto.HexToECDSA(strings.TrimSpace(string(raw)))
		if err != nil {
			return errors.New("Invalid legacy wallet")
		}
		defer wipeKey(pk)
		if !strings.EqualFold(crypto.PubkeyToAddress(pk.PublicKey).Hex(), record.Wallet) {
			return errors.New("Legacy wallet address does not match")
		}
		secret := []byte(legacy.ModelAPIKey)
		defer wipe(secret)
		record, err = encryptRecord(record, pk, secret, password, a.scryptN, a.scryptP)
		if err != nil {
			return errors.New("Encryption failed; legacy wallet kept")
		}
		encrypted, _ := json.Marshal(record)
		if replacePrivate(a.agentFile(record.ID, ".json"), encrypted, a.agentDir) != nil {
			return errors.New("Could not confirm encrypted wallet on disk; restart before retrying")
		}
		a.mu.Lock()
		a.agents[record.ID] = record
		a.mu.Unlock()
	} else {
		key, secret, err := decryptRecord(record, password)
		if err != nil {
			return errWalletPassword
		}
		wipeKey(key.PrivateKey)
		wipe(secret)
	}
	a.mu.Lock()
	a.agents[record.ID] = record
	a.mu.Unlock()
	// A crash here leaves a valid encrypted record and the old .key. Startup
	// flags it for cleanup; repeating migration verifies the password first.
	if err := os.Remove(a.agentFile(record.ID, ".key")); err != nil && !os.IsNotExist(err) {
		return errors.New("Wallet encrypted; could not remove old key file. Run migration again")
	}
	if syncDirectory(a.agentDir) != nil {
		return errors.New("Wallet encrypted; restart and check migration status")
	}
	return nil
}
