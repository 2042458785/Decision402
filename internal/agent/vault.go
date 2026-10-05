package agent

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	evm "github.com/x402-foundation/x402/go/v2/mechanisms/evm"
)

const walletUnlockTTL = 10 * time.Minute

var errWalletLocked = errors.New("Wallet is locked or unlock expired; unlock it again")
var errWalletPassword = errors.New("Wrong password or damaged wallet backup")

// Clear buffers we own. Go and dependencies may retain copies; this is not a
// guarantee against process-memory inspection, swap, or a compromised host.
func wipe(b []byte) { clear(b); runtime.KeepAlive(b) }
func wipeKey(k *ecdsa.PrivateKey) {
	if k != nil && k.D != nil {
		clear(k.D.Bits())
		k.D.SetInt64(0)
		runtime.KeepAlive(k)
	}
}

// Validate before geth's decoder: KDF parameters are untrusted and otherwise
// allow excessive memory use or malformed-input panics. Support bounded V3
// scrypt keystores (including geth StandardScrypt and LightScrypt).
func validCrypto(c keystore.CryptoJSON, size int) bool {
	validHex := func(s string, n int) bool { b, e := hex.DecodeString(s); return e == nil && len(b) == n }
	number := func(name string) int {
		if i, ok := c.KDFParams[name].(int); ok {
			return i
		}
		f, ok := c.KDFParams[name].(float64)
		if !ok || f < 0 || f > 1<<20 || f != float64(int(f)) {
			return -1
		}
		return int(f)
	}
	n, p := number("n"), number("p")
	salt, ok := c.KDFParams["salt"].(string)
	return c.Cipher == "aes-128-ctr" && c.KDF == "scrypt" && number("dklen") == 32 && number("r") == 8 && n >= 2 && n <= keystore.StandardScryptN && n&(n-1) == 0 && p >= 1 && p <= 6 && n*p <= keystore.StandardScryptN && ok && validHex(salt, 32) && validHex(c.CipherParams.IV, 16) && validHex(c.MAC, 32) && validHex(c.CipherText, size)
}
func decryptWallet(data []byte, password string) (*keystore.Key, error) {
	if len(data) > 16384 {
		return nil, errWalletPassword
	}
	var envelope struct {
		Version int                 `json:"version"`
		ID      string              `json:"id"`
		Address string              `json:"address"`
		Crypto  keystore.CryptoJSON `json:"crypto"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Version != 3 || !validCrypto(envelope.Crypto, 32) {
		return nil, errWalletPassword
	}
	if _, err := uuid.Parse(envelope.ID); err != nil {
		return nil, errWalletPassword
	}
	key, err := keystore.DecryptKey(data, password)
	if err != nil {
		return nil, errWalletPassword
	}
	if !strings.EqualFold(envelope.Address, strings.TrimPrefix(key.Address.Hex(), "0x")) {
		wipeKey(key.PrivateKey)
		return nil, errWalletPassword
	}
	return key, nil
}

func encryptRecord(record agentRecord, pk *ecdsa.PrivateKey, modelKey []byte, password string, n, p int) (agentRecord, error) {
	record.Version = 1
	key := &keystore.Key{Id: uuid.New(), Address: crypto.PubkeyToAddress(pk.PublicKey), PrivateKey: pk}
	data, err := keystore.EncryptKey(key, password, n, p)
	if err != nil {
		return record, errors.New("Could not encrypt wallet")
	}
	// Verify before replacing an existing plaintext wallet.
	restored, err := decryptWallet(data, password)
	if err != nil {
		return record, err
	}
	matches := restored.Address == key.Address
	wipeKey(restored.PrivateKey)
	if !matches {
		return record, errors.New("Wallet encryption verification failed")
	}
	record.KeyStore = data
	record.ModelSecret = nil
	if len(modelKey) > 0 {
		auth := []byte(password)
		defer wipe(auth)
		encrypted, err := keystore.EncryptDataV3(modelKey, auth, n, p)
		if err != nil {
			return record, errors.New("Could not encrypt model key")
		}
		verified, err := keystore.DecryptDataV3(encrypted, password)
		if err != nil {
			return record, errors.New("Model key encryption verification failed")
		}
		matches := bytes.Equal(verified, modelKey)
		wipe(verified)
		if !matches {
			return record, errors.New("Model key encryption verification failed")
		}
		record.ModelSecret = &encrypted
	}
	return record, nil
}
func decryptRecord(record agentRecord, password string) (*keystore.Key, []byte, error) {
	key, err := decryptWallet(record.KeyStore, password)
	if err != nil {
		return nil, nil, err
	}
	if !strings.EqualFold(key.Address.Hex(), record.signingAddress()) {
		wipeKey(key.PrivateKey)
		return nil, nil, errWalletPassword
	}
	var secret []byte
	if record.ModelSecret != nil {
		size := len(record.ModelSecret.CipherText) / 2
		if size < 1 || size > 256 || !validCrypto(*record.ModelSecret, size) {
			wipeKey(key.PrivateKey)
			return nil, nil, errWalletPassword
		}
		secret, err = keystore.DecryptDataV3(*record.ModelSecret, password)
		if err != nil {
			wipeKey(key.PrivateKey)
			return nil, nil, errWalletPassword
		}
	}
	return key, secret, nil
}

// During an unlock window, keep an AES-GCM sealed copy plus a random RAM-only
// wrapping key, never the user's password. After unlock, plain private keys are opened only
// inside SignTypedData and immediately cleared. This is still server custody.
type walletGrant struct {
	mu                      sync.Mutex
	wrapping, wallet, model []byte
	session                 string
	expires                 time.Time
	timer                   *time.Timer
}

func sealMemory(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plain, nil), nil
}
func openMemory(key, sealed []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errWalletLocked
	}
	aead, err := cipher.NewGCM(block)
	if err != nil || len(sealed) < aead.NonceSize() {
		return nil, errWalletLocked
	}
	return aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], nil)
}
func newGrant(pk *ecdsa.PrivateKey, model []byte, session string, expires time.Time) (*walletGrant, error) {
	g := &walletGrant{wrapping: make([]byte, 32), session: session, expires: expires}
	if _, err := rand.Read(g.wrapping); err != nil {
		return nil, err
	}
	raw := crypto.FromECDSA(pk)
	defer wipe(raw)
	var err error
	g.wallet, err = sealMemory(g.wrapping, raw)
	if err == nil {
		g.model, err = sealMemory(g.wrapping, model)
	}
	if err != nil {
		g.lock()
		return nil, errors.New("Could not unlock wallet")
	}
	g.mu.Lock()
	g.timer = time.AfterFunc(time.Until(expires), g.lock)
	g.mu.Unlock()
	return g, nil
}
func (g *walletGrant) lock() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.timer != nil {
		g.timer.Stop()
	}
	wipe(g.wrapping)
	wipe(g.wallet)
	wipe(g.model)
	g.wrapping = nil
	g.wallet = nil
	g.model = nil
}
func (g *walletGrant) valid(session string) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.wrapping) == 32 && g.session == session && time.Now().Before(g.expires)
}
func (g *walletGrant) modelKey() ([]byte, error) {
	if g == nil {
		return nil, errWalletLocked
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.wrapping) != 32 || !time.Now().Before(g.expires) {
		return nil, errWalletLocked
	}
	return openMemory(g.wrapping, g.model)
}

type walletSigner struct {
	failureMu  sync.Mutex
	failure    string
	smart      *smartPayment
	grant      *walletGrant
	address    string
	beforeSign func() error
	used       bool
}

func (s *walletSigner) Address() string { return s.address }
func (s *walletSigner) SignTypedData(ctx context.Context, domain evm.TypedDataDomain, types map[string][]evm.TypedDataField, primary string, message map[string]interface{}) ([]byte, error) {
	if s.smart != nil {
		signature, err := s.signSmart(ctx, domain, types, primary, message)
		if err != nil {
			s.failureMu.Lock()
			s.failure = err.Error()
			s.failureMu.Unlock()
		}
		return signature, err
	}
	g := s.grant
	if g == nil {
		return nil, errWalletLocked
	}
	g.mu.Lock()
	defer g.mu.Unlock() // Lock cannot return while a new signature is being created.
	if len(g.wrapping) != 32 || !time.Now().Before(g.expires) || ctx.Err() != nil || s.used {
		return nil, errWalletLocked
	}
	digest, err := evm.HashTypedData(domain, types, primary, message)
	if err != nil {
		return nil, errors.New("Invalid payment authorization")
	}
	if s.beforeSign == nil || s.beforeSign() != nil {
		return nil, errors.New("Could not record payment authorization")
	}
	s.used = true
	// Disk sync can be slow: recheck the deadline before opening the key.
	if ctx.Err() != nil || !time.Now().Before(g.expires) {
		return nil, errWalletLocked
	}
	raw, err := openMemory(g.wrapping, g.wallet)
	if err != nil {
		return nil, errWalletLocked
	}
	defer wipe(raw)
	pk, err := crypto.ToECDSA(raw)
	if err != nil {
		return nil, errWalletLocked
	}
	defer wipeKey(pk)
	if !strings.EqualFold(crypto.PubkeyToAddress(pk.PublicKey).Hex(), s.address) {
		return nil, errWalletLocked
	}
	sig, err := crypto.Sign(digest, pk)
	if err != nil {
		return nil, errors.New("Could not sign payment")
	}
	sig[64] += 27
	return sig, nil
}

func readPrivate(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 32768 {
		return nil, errors.New("Wallet file must be a regular private file (0600)")
	}
	return os.ReadFile(path)
}
func syncDirectory(path string) error {
	d, e := os.Open(path)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func replacePrivate(path string, value []byte, dir string) error {
	f, err := os.CreateTemp(dir, ".encrypted-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(value); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDirectory(dir)
}
