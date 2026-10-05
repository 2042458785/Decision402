package agent

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	x402 "github.com/x402-foundation/x402/go/v2"
	evm "github.com/x402-foundation/x402/go/v2/mechanisms/evm"
	buyer "github.com/x402-foundation/x402/go/v2/mechanisms/evm/exact/client"
	facilitator "github.com/x402-foundation/x402/go/v2/mechanisms/evm/exact/facilitator"
	"github.com/x402-foundation/x402/go/v2/types"
)

func TestSmartMetadataAndRestrictedSigning(t *testing.T) {
	f := newWalletTestApp(t)
	input := testAgentInput()
	input.WalletKind = "" // New API requests default to smart wallets.
	expectStatus(t, f.call(t, "/api/agents", input, f.cookie), 503)
	if err := f.app.EnableSmartWallets("http://127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	w := f.call(t, "/api/agents", input, f.cookie)
	expectStatus(t, w, 201)
	var v agentView
	json.Unmarshal(w.Body.Bytes(), &v)
	if v.Wallet != "" || v.WalletKind != "smart" || !addressPattern.MatchString(v.SessionAddress) {
		t.Fatal("pending wallet must not expose session key as deposit address")
	}
	record := f.app.agents[v.ID]
	key, secret, err := decryptRecord(record, testWalletPassword)
	if err != nil {
		t.Fatal(err)
	}
	defer wipeKey(key.PrivateKey)
	wipe(secret)
	if key.Address.Hex() != v.SessionAddress {
		t.Fatal("encrypted key is not the session key")
	}
	reloaded, err := NewApp(f.app.host, f.app.dir, f.app.model, f.app.scanner, testPayTo, testRisk)
	if err != nil || reloaded.agents[v.ID].SessionAddress != v.SessionAddress {
		t.Fatalf("pending wallet reload: %v", err)
	}
	expectStatus(t, f.action(t, v.ID, "unlock", testWalletPassword, f.cookie), 200)
	signer := &walletSigner{grant: f.app.grants[v.ID], smart: &smartPayment{chain: f.app.smartChain, record: record}}
	if _, err := testSign(signer); err == nil {
		t.Fatal("arbitrary typed data accepted")
	}
	for _, action := range []string{"deploy", "link", "authorize", "revoke", "withdraw"} {
		expectStatus(t, f.call(t, "/api/agents/"+v.ID+"/smart/"+action, map[string]string{}, f.other), 404)
	}
	backup := f.action(t, v.ID, "backup", testWalletPassword, f.cookie)
	expectStatus(t, backup, 200)
	restored := newWalletTestApp(t)
	restored.app.EnableSmartWallets("http://127.0.0.1:1")
	input.WalletKind = "smart"
	input.KeyStore = backup.Body.Bytes()
	result := restored.call(t, "/api/agents/restore", input, restored.cookie)
	expectStatus(t, result, 201)
	var rv agentView
	json.Unmarshal(result.Body.Bytes(), &rv)
	if rv.SessionAddress != v.SessionAddress || rv.Wallet != "" {
		t.Fatal("session restore must require relinking the owner's contract")
	}
}

// Run explicitly after `forge build`: DECISION402_SMART_TEST=1 go test ./internal/agent.
// This starts an isolated local chain. It never connects to a public RPC.
func startWalletChain(t *testing.T) *ethclient.Client {
	t.Helper()
	bin, err := exec.LookPath("anvil")
	if err != nil {
		t.Fatal("Install Foundry/anvil to run smart wallet integration tests")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	command := exec.Command(bin, "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--chain-id", "84532", "--silent")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })
	rpc, err := ethclient.Dial("http://127.0.0.1:" + strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rpc.Close)
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_, err = rpc.ChainID(ctx)
		cancel()
		if err == nil {
			return rpc
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("local chain did not start")
	return nil
}
func localWalletTx(ctx context.Context, rpc *ethclient.Client, key *ecdsa.PrivateKey, to *common.Address, data []byte) (*gethtypes.Receipt, error) {
	nonce, err := rpc.PendingNonceAt(ctx, crypto.PubkeyToAddress(key.PublicKey))
	if err != nil {
		return nil, err
	}
	price, err := rpc.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	tx := gethtypes.NewTx(&gethtypes.LegacyTx{Nonce: nonce, To: to, Gas: 4000000, GasPrice: price, Value: big.NewInt(0), Data: data})
	signed, err := gethtypes.SignTx(tx, gethtypes.LatestSignerForChainID(big.NewInt(84532)), key)
	if err != nil {
		return nil, err
	}
	if err = rpc.SendTransaction(ctx, signed); err != nil {
		return nil, err
	}
	for i := 0; i < 100; i++ {
		r, e := rpc.TransactionReceipt(ctx, signed.Hash())
		if e == nil {
			if r.Status != 1 {
				return r, errors.New("transaction reverted")
			}
			return r, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, errors.New("local transaction not mined")
}

type localWalletFacilitator struct {
	evm.FacilitatorEvmSigner
	rpc *ethclient.Client
	key *ecdsa.PrivateKey
}

func (f *localWalletFacilitator) WriteContract(ctx context.Context, address string, rawABI []byte, name string, suffix []byte, args ...interface{}) (string, error) {
	parsed, err := abi.JSON(bytes.NewReader(rawABI))
	if err != nil {
		return "", err
	}
	data, err := parsed.Pack(name, args...)
	if err != nil {
		return "", err
	}
	to := common.HexToAddress(address)
	r, err := localWalletTx(ctx, f.rpc, f.key, &to, append(data, suffix...))
	if err != nil {
		return "", err
	}
	return r.TxHash.Hex(), nil
}
func settleLocalPayload(ctx context.Context, f *localWalletFacilitator, payload map[string]interface{}) (string, error) {
	b, _ := json.Marshal(payload)
	var p evm.ExactEIP3009Payload
	if err := json.Unmarshal(b, &p); err != nil {
		return "", err
	}
	sig, err := hexutil.Decode(p.Signature)
	if err != nil {
		return "", err
	}
	if len(sig) != 66 {
		return "", errors.New("smart signature did not select bytes overload")
	}
	parsed, err := facilitator.ParseEIP3009Authorization(p.Authorization)
	if err != nil {
		return "", err
	}
	return facilitator.ExecuteTransferWithAuthorization(ctx, f, Asset, parsed, &evm.ERC6492SignatureData{InnerSignature: sig}, nil)
}
func TestSmartWalletIntegration(t *testing.T) {
	if os.Getenv("DECISION402_SMART_TEST") != "1" {
		t.Skip("set DECISION402_SMART_TEST=1 after forge build to run local-chain tests")
	}
	rpc := startWalletChain(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	raw, err := os.ReadFile("../../contracts/out/DecisionWallet.t.sol/MockUSDC.json")
	if err != nil {
		t.Fatal("run forge build --root contracts first")
	}
	var mock struct {
		ABI     json.RawMessage `json:"abi"`
		Runtime struct {
			Object string `json:"object"`
		} `json:"deployedBytecode"`
	}
	if err = json.Unmarshal(raw, &mock); err != nil {
		t.Fatal(err)
	}
	var ignored any
	if err = rpc.Client().CallContext(ctx, &ignored, "anvil_setCode", Asset, mock.Runtime.Object); err != nil {
		t.Fatal(err)
	}
	tokenABI, err := abi.JSON(bytes.NewReader(mock.ABI))
	if err != nil {
		t.Fatal(err)
	}
	ownerKey, _ := crypto.GenerateKey()
	defer wipeKey(ownerKey)
	owner := crypto.PubkeyToAddress(ownerKey.PublicKey)
	if err = rpc.Client().CallContext(ctx, &ignored, "anvil_setBalance", owner.Hex(), "0xde0b6b3a7640000"); err != nil {
		t.Fatal(err)
	}
	f := newWalletTestApp(t)
	f.app.smartChain = &smartChain{rpc: rpc}
	f.app.auth.sessions["owner-session"] = session{address: owner.Hex(), expires: time.Now().Add(time.Hour)}
	input := testAgentInput()
	input.WalletKind = "smart"
	w := f.call(t, "/api/agents", input, f.cookie)
	expectStatus(t, w, 201)
	var view agentView
	json.Unmarshal(w.Body.Bytes(), &view)
	path := "/api/agents/" + view.ID + "/smart/"
	w = f.call(t, path+"deploy", map[string]string{}, f.cookie)
	expectStatus(t, w, 200)
	var tx map[string]string
	json.Unmarshal(w.Body.Bytes(), &tx)
	receipt, err := localWalletTx(ctx, rpc, ownerKey, nil, common.FromHex(tx["data"]))
	if err != nil {
		t.Fatal(err)
	}
	address := receipt.ContractAddress
	expectStatus(t, f.call(t, path+"link", map[string]string{"address": Asset}, f.cookie), 400)
	w = f.call(t, path+"link", map[string]string{"address": address.Hex()}, f.cookie)
	expectStatus(t, w, 200)
	if err = rpc.Client().CallContext(ctx, &ignored, "anvil_setBalance", view.SessionAddress, "0xde0b6b3a7640000"); err != nil {
		t.Fatal(err)
	}
	token := common.HexToAddress(Asset)
	mint, _ := tokenABI.Pack("mint", address, big.NewInt(1000000))
	if _, err = localWalletTx(ctx, rpc, ownerKey, &token, mint); err != nil {
		t.Fatal(err)
	}
	authorize := func() {
		t.Helper()
		w = f.call(t, path+"authorize", map[string]any{"recipients": []string{testPayTo}, "per_payment": "0.01", "daily_limit": "0.02", "expires_at": time.Now().Unix() + 3600}, f.cookie)
		expectStatus(t, w, 200)
		json.Unmarshal(w.Body.Bytes(), &tx)
		if _, err = localWalletTx(ctx, rpc, ownerKey, &address, common.FromHex(tx["data"])); err != nil {
			t.Fatal(err)
		}
	}
	authorize()
	expectStatus(t, f.action(t, view.ID, "unlock", testWalletPassword, f.cookie), 200)
	record := f.app.agents[view.ID]
	journalCalls := 0
	newSigner := func() *walletSigner {
		return &walletSigner{address: address.Hex(), grant: f.app.grants[view.ID], beforeSign: func() error { return nil }, smart: &smartPayment{chain: f.app.smartChain, record: record, beforeBroadcast: func(hash, digest string) error {
			if len(hash) != 66 || len(digest) != 66 {
				return errors.New("missing reservation journal")
			}
			journalCalls++
			return nil
		}}}
	}
	q := testQuote("10000")
	// Gas and journal failures occur before a USDC authorization is signed.
	s := newSigner()
	attempted := 0
	s.beforeSign = func() error { attempted++; return nil }
	if err = rpc.Client().CallContext(ctx, &ignored, "anvil_setBalance", view.SessionAddress, "0x0"); err != nil {
		t.Fatal(err)
	}
	if _, err = buyer.NewExactEvmScheme(s, nil).CreatePaymentPayload(ctx, *q, x402.PaymentPayloadContext{}); err == nil || attempted != 0 || journalCalls != 0 {
		t.Fatalf("gas failure marked a payment attempt: %v attempts=%d", err, attempted)
	}
	if err = rpc.Client().CallContext(ctx, &ignored, "anvil_setBalance", view.SessionAddress, "0xde0b6b3a7640000"); err != nil {
		t.Fatal(err)
	}
	s = newSigner()
	s.beforeSign = func() error { attempted++; return nil }
	s.smart.beforeBroadcast = func(string, string) error { return errors.New("disk unavailable") }
	if _, err = buyer.NewExactEvmScheme(s, nil).CreatePaymentPayload(ctx, *q, x402.PaymentPayloadContext{}); err == nil || attempted != 0 {
		t.Fatal("reservation broadcast without its journal")
	}
	reserved, err := f.app.smartChain.call(ctx, address, "reservedByDay", big.NewInt(time.Now().Unix()/86400))
	if err != nil || reserved[0].(*big.Int).Sign() != 0 {
		t.Fatal("failed journal changed the onchain budget")
	}
	fac := &localWalletFacilitator{rpc: rpc, key: ownerKey}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PAYMENT-SIGNATURE") == "" {
			b, _ := json.Marshal(types.PaymentRequired{X402Version: 2, Accepts: []types.PaymentRequirements{*q}})
			w.Header().Set("PAYMENT-REQUIRED", base64.StdEncoding.EncodeToString(b))
			w.WriteHeader(402)
			fmt.Fprint(w, "{}")
			return
		}
		b, e := base64.StdEncoding.DecodeString(r.Header.Get("PAYMENT-SIGNATURE"))
		var p types.PaymentPayload
		if e != nil || json.Unmarshal(b, &p) != nil {
			http.Error(w, "bad payment", 400)
			return
		}
		hash, e := settleLocalPayload(r.Context(), fac, p.Payload)
		if e != nil {
			http.Error(w, "settlement rejected", 400)
			return
		}
		b, _ = json.Marshal(map[string]any{"success": true, "network": Network, "transaction": hash})
		w.Header().Set("PAYMENT-RESPONSE", base64.StdEncoding.EncodeToString(b))
		fmt.Fprint(w, `{"sample_only":true}`)
	}))
	defer server.Close()
	outcome := Pay(ctx, Candidate{Service: Service{ID: "D", URL: server.URL, PayTo: testPayTo, Amount: "10000"}, Quote: q, Level: 0}, Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 0, Preference: "price"}, newSigner(), &scannerStub{action: "allow"}, func() error { return nil }, func(string, any) {})
	if !outcome.Signed || !outcome.Settled || journalCalls != 1 {
		t.Fatalf("smart x402 roundtrip failed: %+v journal=%d", outcome, journalCalls)
	}
	// Keep the next payment signed but not settled. Revocation must stop it.
	scheme := buyer.NewExactEvmScheme(newSigner(), nil)
	payload, err := scheme.CreatePaymentPayload(ctx, *q, x402.PaymentPayloadContext{})
	if err != nil {
		t.Fatal(err)
	}
	w = f.call(t, path+"revoke", map[string]string{}, f.cookie)
	expectStatus(t, w, 200)
	json.Unmarshal(w.Body.Bytes(), &tx)
	if _, err = localWalletTx(ctx, rpc, ownerKey, &address, common.FromHex(tx["data"])); err != nil {
		t.Fatal(err)
	}
	if _, err = settleLocalPayload(ctx, fac, payload.Payload); err == nil {
		t.Fatal("revoked authorization settled")
	}
	authorize()
	if _, err = buyer.NewExactEvmScheme(newSigner(), nil).CreatePaymentPayload(ctx, *q, x402.PaymentPayloadContext{}); err == nil {
		t.Fatal("reauthorization reset daily spending")
	}
	// Even the actual session key cannot withdraw.
	key, secret, err := decryptRecord(record, testWalletPassword)
	if err != nil {
		t.Fatal(err)
	}
	wipe(secret)
	defer wipeKey(key.PrivateKey)
	withdraw, _ := walletABI.Pack("withdraw", big.NewInt(1))
	if _, err = localWalletTx(ctx, rpc, key.PrivateKey, &address, withdraw); err == nil {
		t.Fatal("session key withdrew owner funds")
	}
	w = f.call(t, path+"withdraw", map[string]string{"amount": "0.99"}, f.cookie)
	expectStatus(t, w, 200)
	json.Unmarshal(w.Body.Bytes(), &tx)
	if tx["to"] != address.Hex() || tx["from"] != owner.Hex() {
		t.Fatal("withdraw not routed to owner contract")
	}
	if _, err = localWalletTx(ctx, rpc, ownerKey, &address, common.FromHex(tx["data"])); err != nil {
		t.Fatal(err)
	}
	balanceData, _ := tokenABI.Pack("balanceOf", owner)
	var balanceHex string
	if err = rpc.Client().CallContext(ctx, &balanceHex, "eth_call", map[string]string{"to": Asset, "data": hexutil.Encode(balanceData)}, "latest"); err != nil {
		t.Fatal(err)
	}
	if new(big.Int).SetBytes(common.FromHex(balanceHex)).Int64() != 990000 {
		t.Fatal("withdrawal did not reach owner")
	}
	// Wrong-chain RPC is rejected, with no fallback to an EOA signature.
	if err = rpc.Client().CallContext(ctx, &ignored, "anvil_setChainId", 1); err != nil {
		t.Fatal(err)
	}
	if err = f.app.smartChain.verify(ctx, record); err == nil {
		t.Fatal("wrong chain accepted")
	}
}

func TestSmartFilterReasonCannotBeRankedBackIn(t *testing.T) {
	cs := []Candidate{{Service: Service{ID: "A"}, Quote: testQuote("1"), Level: 0, WalletReason: "Outside whitelist"}, {Service: Service{ID: "B"}, Quote: testQuote("10000"), Level: 0}}
	out, winner := Rank(Policy{PerPayment: "0.1", TaskBudget: "0.1", Preference: "price", MaxRisk: 0}, cs)
	if winner == nil || winner.Service.ID != "B" || out[0].Eligible || !strings.Contains(out[0].Reason, "whitelist") {
		t.Fatal("wallet refusal ignored")
	}
}
