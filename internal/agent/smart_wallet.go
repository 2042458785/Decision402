package agent

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
)

//go:embed contracts/DecisionWallet.json
var walletArtifact []byte
var walletABI abi.ABI
var walletCreation, walletRuntime []byte

func init() {
	var artifact struct {
		ABI      json.RawMessage `json:"abi"`
		Bytecode string          `json:"bytecode"`
		Runtime  string          `json:"runtime"`
	}
	if json.Unmarshal(walletArtifact, &artifact) != nil {
		panic("invalid wallet artifact")
	}
	var err error
	walletABI, err = abi.JSON(bytes.NewReader(artifact.ABI))
	if err != nil {
		panic(err)
	}
	walletCreation = common.FromHex(artifact.Bytecode)
	walletRuntime = common.FromHex(artifact.Runtime)
	if len(walletCreation) == 0 || len(walletRuntime) == 0 {
		panic("empty wallet artifact")
	}
}
func (r agentRecord) signingAddress() string {
	if r.WalletKind == "smart" {
		return r.SessionAddress
	}
	return r.Wallet
}
func validWalletRecord(r agentRecord) bool {
	if r.WalletKind == "smart" {
		return r.Version == 1 && addressPattern.MatchString(r.SessionAddress) && (r.Wallet == "" || addressPattern.MatchString(r.Wallet)) && !strings.EqualFold(r.Owner, r.SessionAddress)
	}
	return r.WalletKind == "" && r.SessionAddress == "" && addressPattern.MatchString(r.Wallet)
}

type smartChain struct{ rpc *ethclient.Client }

func (a *App) EnableSmartWallets(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) || u.Host == "" {
		return errors.New("invalid RPC URL")
	}
	client, err := ethclient.Dial(rawURL)
	if err != nil {
		return errors.New("RPC connection failed")
	}
	a.smartChain = &smartChain{rpc: client}
	return nil
}
func (c *smartChain) checkNetwork(ctx context.Context) error {
	if c == nil {
		return errors.New("Smart wallet RPC is not configured")
	}
	chain, err := c.rpc.ChainID(ctx)
	if err != nil || chain.Cmp(big.NewInt(84532)) != 0 {
		return errors.New("RPC must be available on Base Sepolia")
	}
	return nil
}
func (c *smartChain) call(ctx context.Context, address common.Address, method string, args ...any) ([]any, error) {
	data, err := walletABI.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	result, err := c.rpc.CallContract(ctx, ethereum.CallMsg{To: &address, Data: data}, nil)
	if err != nil {
		return nil, errors.New("Could not read wallet contract")
	}
	values, err := walletABI.Unpack(method, result)
	if err != nil {
		return nil, errors.New("Invalid wallet response")
	}
	return values, nil
}
func (c *smartChain) verify(ctx context.Context, r agentRecord) error {
	if err := c.checkNetwork(ctx); err != nil {
		return err
	}
	if r.WalletKind != "smart" || !addressPattern.MatchString(r.Wallet) {
		return errors.New("Deploy and link the smart wallet first")
	}
	address := common.HexToAddress(r.Wallet)
	code, err := c.rpc.CodeAt(ctx, address, nil)
	if err != nil || !bytes.Equal(code, walletRuntime) {
		return errors.New("Address is not the supported Decision402 wallet")
	}
	o, err := c.call(ctx, address, "owner")
	if err != nil {
		return err
	}
	t, err := c.call(ctx, address, "token")
	if err != nil {
		return err
	}
	if o[0].(common.Address) != common.HexToAddress(r.Owner) || t[0].(common.Address) != common.HexToAddress(Asset) {
		return errors.New("Wallet owner or USDC contract does not match")
	}
	return nil
}
func (a *App) smartStatus(w http.ResponseWriter, r *http.Request) {
	record, ok := a.ownedAgent(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := a.smartChain.verify(ctx, record); err != nil {
		jsonResponse(w, 409, map[string]string{"error": err.Error()})
		return
	}
	address := common.HexToAddress(record.Wallet)
	result := map[string]any{"wallet": record.Wallet, "session_address": record.SessionAddress}
	for _, name := range []string{"sessionKey", "perPayment", "dailyLimit", "expiresAt", "epoch", "getRecipients"} {
		values, err := a.smartChain.call(ctx, address, name)
		if err != nil {
			jsonResponse(w, 503, map[string]string{"error": err.Error()})
			return
		}
		switch v := values[0].(type) {
		case *big.Int:
			result[name] = v.String()
		default:
			result[name] = v
		}
	}
	header, err := a.smartChain.rpc.HeaderByNumber(ctx, nil)
	if err != nil {
		jsonResponse(w, 503, map[string]string{"error": "Could not read chain time"})
		return
	}
	day := new(big.Int).SetUint64(header.Time / 86400)
	spent, err := a.smartChain.call(ctx, address, "reservedByDay", day)
	if err != nil {
		jsonResponse(w, 503, map[string]string{"error": err.Error()})
		return
	}
	gas, err := a.smartChain.rpc.BalanceAt(ctx, common.HexToAddress(record.SessionAddress), nil)
	if err != nil {
		jsonResponse(w, 503, map[string]string{"error": "Could not read session gas balance"})
		return
	}
	result["reserved_today"] = spent[0].(*big.Int).String()
	result["utc_day"] = day.String()
	result["gas_wei"] = gas.String()
	result["chain_time"] = header.Time
	jsonResponse(w, 200, result)
}
func (a *App) smartAction(w http.ResponseWriter, r *http.Request) {
	record, ok := a.ownedAgent(w, r)
	if !ok {
		return
	}
	if record.WalletKind != "smart" {
		jsonResponse(w, 409, map[string]string{"error": "Create a new smart wallet; an existing EOA cannot change its address"})
		return
	}
	var input struct {
		Address    string   `json:"address"`
		Recipients []string `json:"recipients"`
		PerPayment string   `json:"per_payment"`
		DailyLimit string   `json:"daily_limit"`
		ExpiresAt  int64    `json:"expires_at"`
		Amount     string   `json:"amount"`
	}
	if !readSecretRequest(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	action := r.PathValue("action")
	if err := a.smartChain.checkNetwork(ctx); err != nil {
		jsonResponse(w, 503, map[string]string{"error": err.Error()})
		return
	}
	if action == "deploy" {
		if record.Wallet != "" {
			jsonResponse(w, 409, map[string]string{"error": "Wallet is already linked"})
			return
		}
		args, _ := walletABI.Pack("", common.HexToAddress(record.Owner), common.HexToAddress(Asset))
		data := append(append([]byte{}, walletCreation...), args...)
		jsonResponse(w, 200, map[string]string{"from": record.Owner, "data": hexutil.Encode(data), "value": "0x0", "chainId": "0x14a34"})
		return
	}
	if action == "link" {
		a.walletOps.Lock()
		defer a.walletOps.Unlock()
		record, ok = a.ownedAgent(w, r)
		if !ok {
			return
		}
		if !addressPattern.MatchString(input.Address) || (record.Wallet != "" && !strings.EqualFold(record.Wallet, input.Address)) {
			jsonResponse(w, 409, map[string]string{"error": "Invalid address or wallet is already linked"})
			return
		}
		record.Wallet = common.HexToAddress(input.Address).Hex()
		if err := a.smartChain.verify(ctx, record); err != nil {
			jsonResponse(w, 400, map[string]string{"error": err.Error()})
			return
		}
		a.mu.Lock()
		duplicate := false
		for id, other := range a.agents {
			if id != record.ID && strings.EqualFold(other.Wallet, record.Wallet) {
				duplicate = true
			}
		}
		a.mu.Unlock()
		if duplicate {
			jsonResponse(w, 409, map[string]string{"error": "Wallet already linked to another agent"})
			return
		}
		b, _ := json.Marshal(record)
		if replacePrivate(a.agentFile(record.ID, ".json"), b, a.agentDir) != nil {
			jsonResponse(w, 500, map[string]string{"error": "Could not confirm linked wallet on disk; restart before retrying"})
			return
		}
		a.mu.Lock()
		a.agents[record.ID] = record
		a.mu.Unlock()
		jsonResponse(w, 200, a.walletView(record, r))
		return
	}
	if err := a.smartChain.verify(ctx, record); err != nil {
		jsonResponse(w, 409, map[string]string{"error": err.Error()})
		return
	}
	var data []byte
	var err error
	switch action {
	case "authorize":
		per, e1 := Money(input.PerPayment)
		daily, e2 := Money(input.DailyLimit)
		header, e3 := a.smartChain.rpc.HeaderByNumber(ctx, nil)
		if e1 != nil || e2 != nil || per <= 0 || per > MaxDemoAtomic || daily < per || daily > 100000000 || e3 != nil || input.ExpiresAt <= int64(header.Time) || input.ExpiresAt > int64(header.Time)+30*86400 || len(input.Recipients) < 1 || len(input.Recipients) > 32 {
			jsonResponse(w, 400, map[string]string{"error": "Use 1-32 recipients, up to 0.10 USDC per payment, up to 100 USDC daily, and an expiry within 30 days"})
			return
		}
		recipients := []common.Address{}
		seen := map[common.Address]bool{}
		for _, v := range input.Recipients {
			p := common.HexToAddress(v)
			if !addressPattern.MatchString(v) || p == (common.Address{}) || p == common.HexToAddress(record.Wallet) || seen[p] {
				jsonResponse(w, 400, map[string]string{"error": "Use distinct, nonzero recipient addresses"})
				return
			}
			seen[p] = true
			recipients = append(recipients, p)
		}
		data, err = walletABI.Pack("authorize", common.HexToAddress(record.SessionAddress), recipients, big.NewInt(per), big.NewInt(daily), big.NewInt(input.ExpiresAt))
	case "revoke":
		data, err = walletABI.Pack("revoke")
	case "withdraw":
		amount, e := Money(input.Amount)
		if e != nil || amount <= 0 || amount > 100000000 {
			jsonResponse(w, 400, map[string]string{"error": "Withdraw more than 0 and at most 100 test USDC"})
			return
		}
		data, err = walletABI.Pack("withdraw", big.NewInt(amount))
	default:
		jsonResponse(w, 404, map[string]string{"error": "Unknown smart wallet action"})
		return
	}
	if err != nil {
		jsonResponse(w, 400, map[string]string{"error": "Invalid wallet transaction"})
		return
	}
	// Only prepare calldata. The owner's MetaMask signs and sends it.
	jsonResponse(w, 200, map[string]string{"from": record.Owner, "to": record.Wallet, "data": hexutil.Encode(data), "value": "0x0", "chainId": "0x14a34"})
}

// Filter for the UI/selection; reservePayment still repeats the checks on chain.
func (p *smartPayment) filter(ctx context.Context, policy Policy, candidates []Candidate) ([]Candidate, *Candidate) {
	reason := ""
	if err := p.chain.verify(ctx, p.record); err != nil {
		reason = err.Error()
	}
	address := common.HexToAddress(p.record.Wallet)
	values := map[string]any{}
	if reason == "" {
		for _, method := range []string{"sessionKey", "perPayment", "dailyLimit", "expiresAt", "getRecipients"} {
			v, err := p.chain.call(ctx, address, method)
			if err != nil {
				reason = "Could not read onchain authorization"
				break
			}
			values[method] = v[0]
		}
	}
	remaining := new(big.Int)
	if reason == "" {
		h, err := p.chain.rpc.HeaderByNumber(ctx, nil)
		if err != nil {
			reason = "Could not read chain time"
		} else if values["sessionKey"].(common.Address) != common.HexToAddress(p.record.SessionAddress) || values["expiresAt"].(*big.Int).Cmp(new(big.Int).SetUint64(h.Time+300)) <= 0 {
			reason = "Authorize this session for at least five more minutes"
		} else {
			spent, err := p.chain.call(ctx, address, "reservedByDay", new(big.Int).SetUint64(h.Time/86400))
			if err != nil {
				reason = "Could not read daily budget"
			} else {
				remaining.Sub(values["dailyLimit"].(*big.Int), spent[0].(*big.Int))
			}
		}
	}
	allowed := map[common.Address]bool{}
	if reason == "" {
		for _, v := range values["getRecipients"].([]common.Address) {
			allowed[v] = true
		}
	}
	for i := range candidates {
		c := &candidates[i]
		c.WalletReason = reason
		if reason != "" || c.Quote == nil {
			continue
		}
		amount, ok := new(big.Int).SetString(c.Quote.Amount, 10)
		if !allowed[common.HexToAddress(c.Quote.PayTo)] {
			c.WalletReason = "Recipient is outside the wallet whitelist"
		} else if !ok || amount.Cmp(values["perPayment"].(*big.Int)) > 0 || amount.Cmp(remaining) > 0 {
			c.WalletReason = "Exceeds the onchain per-payment or remaining daily limit"
		}
	}
	return Rank(policy, candidates)
}
