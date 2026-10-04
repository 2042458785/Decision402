package agent

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto"
	evm "github.com/x402-foundation/x402/go/v2/mechanisms/evm"
)

const testWalletPassword = "test-wallet-password-2026"
const testModelSecret = "private-model-key-xyz-2026"

type walletTestApp struct {
	app           *App
	handler       http.Handler
	cookie, other *http.Cookie
}

func newWalletTestApp(t *testing.T) *walletTestApp {
	t.Helper()
	app, err := NewApp("127.0.0.1:8080", filepath.Join(t.TempDir(), "tasks"), NewModel("server-default-key", "deepseek-flash", "https://api.deepseek.com"), &scannerStub{action: "allow"}, testPayTo, testRisk)
	if err != nil {
		t.Fatal(err)
	}
	app.scryptN, app.scryptP = keystore.LightScryptN, keystore.LightScryptP
	app.auth.sessions["owner-session"] = session{address: testPayTo, expires: time.Now().Add(time.Hour)}
	app.auth.sessions["other-session"] = session{address: testRisk, expires: time.Now().Add(time.Hour)}
	f := &walletTestApp{app: app, handler: app.Handler(t.TempDir()), cookie: &http.Cookie{Name: "decision402_session", Value: "owner-session"}, other: &http.Cookie{Name: "decision402_session", Value: "other-session"}}
	t.Cleanup(func() {
		for id := range app.agents {
			app.lockWallet(id)
		}
	})
	return f
}
func (f *walletTestApp) call(t *testing.T, path string, body any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	r := httptest.NewRequest("POST", "http://127.0.0.1:8080"+path, bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Decision402", "local-ui")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}
func testAgentInput() agentInput {
	return agentInput{Name: "Test buyer", ModelURL: "https://api.deepseek.com", ModelName: "deepseek-flash", ModelAPIKey: testModelSecret, Password: testWalletPassword}
}
func (f *walletTestApp) create(t *testing.T) agentView {
	t.Helper()
	w := f.call(t, "/api/agents", testAgentInput(), f.cookie)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var view agentView
	if json.Unmarshal(w.Body.Bytes(), &view) != nil {
		t.Fatal("invalid view")
	}
	return view
}
func (f *walletTestApp) action(t *testing.T, id, action, password string, cookie *http.Cookie) *httptest.ResponseRecorder {
	return f.call(t, "/api/agents/"+id+"/"+action, map[string]string{"password": password}, cookie)
}
func expectStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("want %d got %d: %s", status, w.Code, w.Body)
	}
}
func testSign(s *walletSigner) ([]byte, error) {
	return s.SignTypedData(context.Background(), evm.TypedDataDomain{Name: "USDC", Version: "2", ChainID: big.NewInt(84532), VerifyingContract: Asset}, map[string][]evm.TypedDataField{"Test": {{Name: "value", Type: "uint256"}}}, "Test", map[string]interface{}{"value": "1"})
}
func TestEncryptedWalletLifecycleAndBackup(t *testing.T) {
	f := newWalletTestApp(t)
	var logs bytes.Buffer
	originalLog := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(originalLog)
	v := f.create(t)
	if !v.Locked || v.NeedsMigration {
		t.Fatal("new wallet should be encrypted and locked")
	}
	record := f.app.agents[v.ID]
	key, secret, err := decryptRecord(record, testWalletPassword)
	if err != nil {
		t.Fatal(err)
	}
	rawKey := hex.EncodeToString(crypto.FromECDSA(key.PrivateKey))
	wipeKey(key.PrivateKey)
	if string(secret) != testModelSecret {
		t.Fatal("personal model key did not survive encryption")
	}
	wipe(secret)
	data, err := os.ReadFile(f.app.agentFile(v.ID, ".json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{testWalletPassword, testModelSecret, rawKey, `"model_api_key"`} {
		if bytes.Contains(data, []byte(s)) {
			t.Fatal("plaintext credential persisted")
		}
	}
	if _, err = os.Stat(f.app.agentFile(v.ID, ".key")); !os.IsNotExist(err) {
		t.Fatal("plaintext key file created")
	}
	for _, action := range []string{"unlock", "backup", "lock", "migrate"} {
		expectStatus(t, f.action(t, v.ID, action, testWalletPassword, f.other), 404)
		expectStatus(t, f.action(t, v.ID, action, testWalletPassword, nil), 401)
	}
	expectStatus(t, f.action(t, v.ID, "unlock", "incorrect-password", f.cookie), 400)
	if f.app.grants[v.ID] != nil {
		t.Fatal("wrong password unlocked wallet")
	}
	payment := TaskRequest{ID: "wallet-locked-task-0001", AgentID: v.ID, Instruction: "Tokyo", Mode: "pay", Policy: Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 0, Preference: "price"}}
	expectStatus(t, f.call(t, "/api/tasks", payment, f.cookie), 423)
	if len(f.app.tasks) != 0 {
		t.Fatal("locked request created a task")
	}
	expectStatus(t, f.action(t, v.ID, "unlock", testWalletPassword, f.cookie), 200)
	grant := f.app.grants[v.ID]
	reserved := 0
	signer := &walletSigner{grant: grant, address: v.Wallet, beforeSign: func() error { reserved++; return nil }}
	sig, err := testSign(signer)
	if err != nil || len(sig) != 65 || reserved != 1 {
		t.Fatalf("unlocked wallet cannot sign: %v", err)
	}
	digest, e := evm.HashTypedData(evm.TypedDataDomain{Name: "USDC", Version: "2", ChainID: big.NewInt(84532), VerifyingContract: Asset}, map[string][]evm.TypedDataField{"Test": {{Name: "value", Type: "uint256"}}}, "Test", map[string]interface{}{"value": "1"})
	if e != nil {
		t.Fatal(e)
	}
	recoveredSig := append([]byte(nil), sig...)
	recoveredSig[64] -= 27
	public, e := crypto.SigToPub(digest, recoveredSig)
	if e != nil || crypto.PubkeyToAddress(*public).Hex() != v.Wallet {
		t.Fatal("signature does not match wallet")
	}
	if _, err = testSign(signer); err == nil || reserved != 1 {
		t.Fatal("same signer signed twice")
	}
	// A different login session belonging to the SAME owner must still unlock.
	f.app.auth.sessions["same-owner-other-session"] = session{address: testPayTo, expires: time.Now().Add(time.Hour)}
	otherSession := &http.Cookie{Name: "decision402_session", Value: "same-owner-other-session"}
	expectStatus(t, f.call(t, "/api/tasks", payment, otherSession), 423)
	expectStatus(t, f.action(t, v.ID, "lock", "", f.cookie), 200)
	stale := &walletSigner{grant: grant, address: v.Wallet, beforeSign: func() error { t.Fatal("locked wallet reserved payment"); return nil }}
	if _, err = testSign(stale); err == nil {
		t.Fatal("lock left old signer active")
	}
	expectStatus(t, f.action(t, v.ID, "unlock", testWalletPassword, f.cookie), 200)
	if _, err = testSign(stale); err == nil {
		t.Fatal("re-unlock revived an old task")
	}
	backup := f.action(t, v.ID, "backup", testWalletPassword, f.cookie)
	expectStatus(t, backup, 200)
	if backup.Header().Get("Cache-Control") != "no-store" || !strings.Contains(backup.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("backup not protected/downloadable")
	}
	if _, err := decryptWallet(backup.Body.Bytes(), "wrong-password"); err == nil {
		t.Fatal("backup accepted wrong password")
	}
	// Restore on a different installation, then actually sign with the restored address.
	restored := newWalletTestApp(t)
	input := testAgentInput()
	input.KeyStore = append([]byte(nil), backup.Body.Bytes()...)
	imported := restored.call(t, "/api/agents/restore", input, restored.cookie)
	expectStatus(t, imported, 201)
	var rv agentView
	json.Unmarshal(imported.Body.Bytes(), &rv)
	if rv.Wallet != v.Wallet || !rv.Locked {
		t.Fatal("backup did not restore the same locked wallet")
	}
	expectStatus(t, restored.action(t, rv.ID, "unlock", testWalletPassword, restored.cookie), 200)
	rs := &walletSigner{grant: restored.app.grants[rv.ID], address: rv.Wallet, beforeSign: func() error { return nil }}
	if _, err := testSign(rs); err != nil {
		t.Fatal("restored wallet cannot sign")
	}
	expectStatus(t, restored.call(t, "/api/agents/restore", input, restored.cookie), 409)
	input.Password = "incorrect-password"
	expectStatus(t, newWalletTestApp(t).call(t, "/api/agents/restore", input, f.cookie), 400)
	restarted, err := NewApp(f.app.host, f.app.dir, f.app.model, f.app.scanner, testPayTo, testRisk)
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.grants) != 0 {
		t.Fatal("unlock survived restart")
	}
	for _, response := range []string{logs.String(), imported.Body.String(), backup.Body.String()} {
		for _, secret := range []string{testWalletPassword, testModelSecret, rawKey} {
			if strings.Contains(response, secret) {
				t.Fatal("response or log leaked credential")
			}
		}
	}
	expectStatus(t, f.call(t, "/api/auth/logout", map[string]string{}, f.cookie), 200)
	if len(f.app.grants) != 0 {
		t.Fatal("logout did not lock wallets")
	}
}
func TestWalletExpiryAndLockDuringSignature(t *testing.T) {
	pk, _ := crypto.GenerateKey()
	defer wipeKey(pk)
	expired, err := newGrant(pk, nil, "test", time.Now().Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer expired.lock()
	if _, err = testSign(&walletSigner{grant: expired}); err == nil {
		t.Fatal("expired grant signed")
	}
	g, err := newGrant(pk, nil, "test", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	defer g.lock()
	entered, release, finished, locked := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	s := &walletSigner{grant: g, address: crypto.PubkeyToAddress(pk.PublicKey).Hex(), beforeSign: func() error { close(entered); <-release; return nil }}
	go func() {
		defer close(finished)
		_, err := testSign(s)
		if err != nil {
			t.Error(err)
		}
	}()
	<-entered
	go func() { g.lock(); close(locked) }()
	select {
	case <-locked:
		t.Fatal("lock returned before in-progress signing finished")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-finished
	<-locked
	if _, err = testSign(&walletSigner{grant: g}); err == nil {
		t.Fatal("signing after lock succeeded")
	}
	if len(g.wrapping) != 0 || len(g.wallet) != 0 || len(g.model) != 0 {
		t.Fatal("lock retained secret buffers")
	}
}
func TestLegacyWalletMigrationAndInterruptedCleanup(t *testing.T) {
	f := newWalletTestApp(t)
	pk, _ := crypto.GenerateKey()
	defer wipeKey(pk)
	id := "legacy-wallet-00001"
	address := crypto.PubkeyToAddress(pk.PublicKey).Hex()
	metadata := map[string]any{"id": id, "owner": testPayTo, "name": "Old wallet", "wallet": address, "model_url": "https://api.deepseek.com", "model_name": "deepseek-flash", "model_api_key": testModelSecret}
	data, _ := json.Marshal(metadata)
	raw := []byte(hex.EncodeToString(crypto.FromECDSA(pk)))
	defer wipe(raw)
	if writePrivate(f.app.agentFile(id, ".json"), data) != nil || writePrivate(f.app.agentFile(id, ".key"), raw) != nil {
		t.Fatal("fixture failed")
	}
	if err := f.app.loadAgents(); err != nil {
		t.Fatal(err)
	}
	expectStatus(t, f.action(t, id, "unlock", testWalletPassword, f.cookie), 409)
	expectStatus(t, f.action(t, id, "migrate", "short", f.cookie), 400)
	before, _ := os.ReadFile(f.app.agentFile(id, ".key"))
	if !bytes.Equal(before, raw) {
		t.Fatal("failed migration changed old key")
	}
	expectStatus(t, f.action(t, id, "migrate", testWalletPassword, f.cookie), 200)
	if _, err := os.Stat(f.app.agentFile(id, ".key")); !os.IsNotExist(err) {
		t.Fatal("migration left plaintext key")
	}
	encrypted, _ := os.ReadFile(f.app.agentFile(id, ".json"))
	if bytes.Contains(encrypted, raw) || bytes.Contains(encrypted, []byte(testModelSecret)) {
		t.Fatal("migration left plaintext secret")
	}
	key, secret, err := decryptRecord(f.app.agents[id], testWalletPassword)
	if err != nil || key.Address.Hex() != address || string(secret) != testModelSecret {
		t.Fatal("migration changed wallet/model key")
	}
	wipeKey(key.PrivateKey)
	wipe(secret)
	// Simulate power loss after encrypted metadata was committed but before .key removal.
	if writePrivate(f.app.agentFile(id, ".key"), raw) != nil {
		t.Fatal("fixture failed")
	}
	expectStatus(t, f.action(t, id, "unlock", testWalletPassword, f.cookie), 409)
	expectStatus(t, f.action(t, id, "migrate", "wrong-password", f.cookie), 400)
	if _, err := os.Stat(f.app.agentFile(id, ".key")); err != nil {
		t.Fatal("wrong password deleted original")
	}
	// Also simulate stale in-memory metadata after a directory-sync failure.
	stale := f.app.agents[id]
	stale.Version = 0
	f.app.agents[id] = stale
	expectStatus(t, f.action(t, id, "migrate", testWalletPassword, f.cookie), 200)
	// Reload verifies both the crash cleanup and the preserved encrypted model key.
	if err := f.app.loadAgents(); err != nil {
		t.Fatal(err)
	}
	key, secret, err = decryptRecord(f.app.agents[id], testWalletPassword)
	if err != nil || string(secret) != testModelSecret {
		t.Fatal("crash recovery lost model key")
	}
	wipeKey(key.PrivateKey)
	wipe(secret)
}
func TestKeystoreRejectsMalformedAndExcessiveKDF(t *testing.T) {
	f := newWalletTestApp(t)
	v := f.create(t)
	original := f.app.agents[v.ID].KeyStore
	for _, mutation := range []func(map[string]any){
		func(v map[string]any) { v["version"] = 1 },
		func(v map[string]any) {
			v["crypto"].(map[string]any)["kdfparams"].(map[string]any)["n"] = float64(1 << 30)
		},
		func(v map[string]any) { v["crypto"].(map[string]any)["kdfparams"].(map[string]any)["dklen"] = 1 },
		func(v map[string]any) { v["crypto"].(map[string]any)["kdfparams"].(map[string]any)["n"] = "bad" },
		func(v map[string]any) { v["crypto"].(map[string]any)["cipherparams"].(map[string]any)["iv"] = "00" },
		func(v map[string]any) { v["address"] = strings.Repeat("0", 40) },
	} {
		var obj map[string]any
		json.Unmarshal(original, &obj)
		mutation(obj)
		data, _ := json.Marshal(obj)
		if key, err := decryptWallet(data, testWalletPassword); err == nil {
			wipeKey(key.PrivateKey)
			t.Fatal("malformed keystore accepted")
		}
	}
}
func TestModelResponseCannotEchoCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": testModelSecret}}}})
		w.Write(bytes.ReplaceAll(b, []byte("private-model"), []byte(`\u0070rivate-model`)))
	}))
	defer server.Close()
	m := NewModel(testModelSecret, "test", server.URL)
	msg, err := m.Complete(context.Background(), nil)
	if err == nil || strings.Contains(err.Error(), testModelSecret) || strings.Contains(msg.Content, testModelSecret) {
		t.Fatal("model credential reflected into result")
	}
}

// A lock received while an earlier unlock is queued must win.
func TestLockCancelsPendingUnlock(t *testing.T) {
	f := newWalletTestApp(t)
	v := f.create(t)
	started := make(chan struct{})
	body := &notifyRead{Reader: strings.NewReader(`{"password":"` + testWalletPassword + `"}`), started: started}
	r := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/agents/"+v.ID+"/unlock", body)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Decision402", "local-ui")
	r.AddCookie(f.cookie)
	done := make(chan struct{})
	w := httptest.NewRecorder()
	f.app.walletOps.Lock()
	go func() { defer close(done); f.handler.ServeHTTP(w, r) }()
	<-started
	locked := f.action(t, v.ID, "lock", "", f.cookie)
	f.app.walletOps.Unlock()
	expectStatus(t, locked, 200)
	<-done
	expectStatus(t, w, 409)
	if f.app.grants[v.ID] != nil {
		t.Fatal("a pending unlock reversed a later lock")
	}
}

type notifyRead struct {
	*strings.Reader
	started  chan struct{}
	notified bool
}

func (r *notifyRead) Read(p []byte) (int, error) {
	if !r.notified {
		r.notified = true
		close(r.started)
	}
	return r.Reader.Read(p)
}

func TestUnlockExpiresWhileJournalIsWritten(t *testing.T) {
	pk, _ := crypto.GenerateKey()
	defer wipeKey(pk)
	g, err := newGrant(pk, nil, "test", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	defer g.lock()
	signer := &walletSigner{grant: g, address: crypto.PubkeyToAddress(pk.PublicKey).Hex(), beforeSign: func() error {
		// Simulate crossing the expiry boundary while syncing the payment journal.
		g.expires = time.Now().Add(-time.Second)
		return nil
	}}
	if sig, err := testSign(signer); err == nil || len(sig) != 0 {
		t.Fatal("expired wallet signed after journal sync")
	}
}
