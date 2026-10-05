package agent

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type withdrawalChainStub struct {
	chainID      *big.Int
	usdc         *big.Int
	eth          *big.Int
	sent         []*types.Transaction
	confirmed    bool
	omitTransfer bool
}

func (s *withdrawalChainStub) ChainID(context.Context) (*big.Int, error) { return s.chainID, nil }
func (s *withdrawalChainStub) CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
	return common.LeftPadBytes(s.usdc.Bytes(), 32), nil
}
func (s *withdrawalChainStub) BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error) {
	return s.eth, nil
}
func (s *withdrawalChainStub) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	return 0, nil
}
func (s *withdrawalChainStub) EstimateGas(context.Context, ethereum.CallMsg) (uint64, error) {
	return 50000, nil
}
func (s *withdrawalChainStub) SuggestGasTipCap(context.Context) (*big.Int, error) {
	return big.NewInt(100000000), nil
}
func (s *withdrawalChainStub) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return &types.Header{BaseFee: big.NewInt(1000000000)}, nil
}
func (s *withdrawalChainStub) SendTransaction(_ context.Context, tx *types.Transaction) error {
	s.sent = append(s.sent, tx)
	return nil
}
func (s *withdrawalChainStub) TransactionReceipt(_ context.Context, hash common.Hash) (*types.Receipt, error) {
	if !s.confirmed {
		return nil, ethereum.NotFound
	}
	sender, _ := types.Sender(types.LatestSignerForChainID(big.NewInt(84532)), s.sent[0])
	if s.omitTransfer {
		return &types.Receipt{TxHash: hash, Status: types.ReceiptStatusSuccessful}, nil
	}
	return &types.Receipt{TxHash: hash, Status: types.ReceiptStatusSuccessful, Logs: []*types.Log{{Address: common.HexToAddress(Asset), Topics: []common.Hash{crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)")), common.BytesToHash(sender.Bytes()), common.BytesToHash(common.BytesToAddress(s.sent[0].Data()[16:36]).Bytes())}, Data: common.LeftPadBytes(new(big.Int).SetBytes(s.sent[0].Data()[36:68]).Bytes(), 32)}}}, nil
}
func (s *withdrawalChainStub) Close() {}

func TestWithdrawalIsOwnerOnlyAndIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tasks")
	a, err := NewApp("127.0.0.1:8080", dir, NewModel("server-key", "deepseek-flash", "https://api.deepseek.com"), &scannerStub{action: "allow"}, testPayTo, testRisk)
	if err != nil {
		t.Fatal(err)
	}
	a.scryptN, a.scryptP = keystore.LightScryptN, keystore.LightScryptP
	ownerKey, _ := crypto.GenerateKey()
	owner := crypto.PubkeyToAddress(ownerKey.PublicKey).Hex()
	agentKey, _ := crypto.GenerateKey()
	wallet := crypto.PubkeyToAddress(agentKey.PublicKey).Hex()
	record := agentRecord{ID: "test-agent-wallet-0001", Owner: owner, Name: "buyer", Wallet: wallet, ModelURL: "https://api.deepseek.com", ModelName: "deepseek-flash"}
	record, err = encryptRecord(record, agentKey, nil, "test-wallet-password", a.scryptN, a.scryptP)
	if err != nil {
		t.Fatal(err)
	}
	a.agents[record.ID] = record
	data, _ := json.Marshal(record)
	if err := os.WriteFile(a.agentFile(record.ID, ".json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	a.auth.sessions["owner-session"] = session{address: owner, expires: time.Now().Add(time.Hour)}
	grant, err := newGrant(agentKey, nil, "owner-session", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	a.grants[record.ID] = grant
	stub := &withdrawalChainStub{chainID: big.NewInt(84532), usdc: big.NewInt(1000000), eth: big.NewInt(1000000000000000000)}
	a.withdrawDial = func(context.Context) (withdrawalChain, error) { return stub, nil }
	handler := a.Handler(t.TempDir())
	path := "/api/agents/" + record.ID + "/withdraw"
	call := func(method, path, body string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, strings.NewReader(body))
		if method == "POST" {
			r.Header.Set("X-Decision402", "local-ui")
			r.Header.Set("Content-Type", "application/json")
		}
		if auth {
			r.AddCookie(&http.Cookie{Name: "decision402_session", Value: "owner-session"})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	body := `{"id":"withdraw-attempt-0001","amount":"0.5","password":"test-wallet-password"}`
	if got := call("POST", path, body, false); got.Code != 401 {
		t.Fatalf("unauthenticated withdrawal: %d", got.Code)
	}
	if got := call("POST", path, `{"id":"withdraw-attempt-0001","amount":"0.5","password":"wrong"}`, true); got.Code != 400 {
		t.Fatalf("wrong password: %d", got.Code)
	}
	stub.chainID = big.NewInt(1)
	if got := call("POST", path, body, true); got.Code != 503 {
		t.Fatalf("wrong chain: %d", got.Code)
	}
	stub.chainID = big.NewInt(84532)
	stub.eth = big.NewInt(0)
	if got := call("POST", path, body, true); got.Code != 400 {
		t.Fatalf("no gas: %d", got.Code)
	}
	stub.eth = big.NewInt(1000000000000000000)
	got := call("POST", path, body, true)
	if got.Code != 202 || strings.Contains(got.Body.String(), "raw_tx") || strings.Contains(got.Body.String(), "test-wallet-password") {
		t.Fatalf("withdrawal response: %d %s", got.Code, got.Body.String())
	}
	if len(stub.sent) != 1 {
		t.Fatalf("expected one send, got %d", len(stub.sent))
	}
	tx := stub.sent[0]
	signer := types.LatestSignerForChainID(big.NewInt(84532))
	from, err := types.Sender(signer, tx)
	if err != nil || !strings.EqualFold(from.Hex(), wallet) || tx.To() == nil || !strings.EqualFold(tx.To().Hex(), Asset) {
		t.Fatal("wrong withdrawal signer or asset")
	}
	if !strings.EqualFold(common.BytesToAddress(tx.Data()[16:36]).Hex(), owner) || new(big.Int).SetBytes(tx.Data()[36:68]).Cmp(big.NewInt(500000)) != 0 {
		t.Fatal("withdrawal did not send 0.5 USDC to owner")
	}
	again := call("POST", path, body, true)
	if again.Code != 200 || len(stub.sent) != 2 || stub.sent[1].Hash() != tx.Hash() {
		t.Fatal("retry created a different transaction")
	}
	other := call("POST", path, `{"id":"withdraw-attempt-0002","amount":"0.5","password":"test-wallet-password"}`, true)
	if other.Code != 409 {
		t.Fatalf("new withdrawal while pending: %d", other.Code)
	}
	reloaded, err := NewApp(a.host, dir, a.model, a.scanner, testPayTo, testRisk)
	if err != nil || reloaded.withdrawals["withdraw-attempt-0001"].Transaction != tx.Hash().Hex() {
		t.Fatalf("withdrawal journal did not reload: %v", err)
	}
	stub.confirmed = true
	stub.omitTransfer = true
	unverified := call("GET", "/api/agents/"+record.ID+"/withdrawals", "", true)
	if unverified.Code != 200 || !strings.Contains(unverified.Body.String(), `"status":"pending"`) {
		t.Fatal("success receipt without USDC transfer was accepted")
	}
	stub.omitTransfer = false
	listed := call("GET", "/api/agents/"+record.ID+"/withdrawals", "", true)
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), `"status":"confirmed"`) || strings.Contains(listed.Body.String(), "raw_tx") {
		t.Fatalf("withdrawal status: %s", listed.Body.String())
	}
	if a.withdrawals["withdraw-attempt-0001"].Status != "confirmed" {
		t.Fatal("confirmation was not saved")
	}
}
