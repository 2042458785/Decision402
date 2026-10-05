package agent

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const defaultWithdrawRPC = "https://sepolia.base.org"

type withdrawal struct {
	ID          string `json:"id"`
	AgentID     string `json:"agent_id"`
	Owner       string `json:"owner"`
	Wallet      string `json:"wallet"`
	To          string `json:"to"`
	Amount      string `json:"amount"`
	Transaction string `json:"transaction"`
	RawTx       string `json:"raw_tx"`
	Status      string `json:"status"`
	Created     string `json:"created"`
}

type withdrawalView struct {
	ID          string `json:"id"`
	AgentID     string `json:"agent_id"`
	To          string `json:"to"`
	Amount      string `json:"amount"`
	Transaction string `json:"transaction"`
	Status      string `json:"status"`
	Created     string `json:"created"`
}

func (x withdrawal) view() withdrawalView {
	return withdrawalView{x.ID, x.AgentID, x.To, x.Amount, x.Transaction, x.Status, x.Created}
}

type withdrawalChain interface {
	ChainID(context.Context) (*big.Int, error)
	CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)
	BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error)
	PendingNonceAt(context.Context, common.Address) (uint64, error)
	EstimateGas(context.Context, ethereum.CallMsg) (uint64, error)
	SuggestGasTipCap(context.Context) (*big.Int, error)
	HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
	SendTransaction(context.Context, *types.Transaction) error
	TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error)
	Close()
}

func (a *App) withdrawalClient(ctx context.Context) (withdrawalChain, error) {
	if a.withdrawDial != nil {
		return a.withdrawDial(ctx)
	}
	return ethclient.DialContext(ctx, a.withdrawRPC)
}

func (a *App) loadWithdrawals() error {
	if err := os.MkdirAll(a.withdrawDir, 0700); err != nil {
		return err
	}
	info, err := os.Stat(a.withdrawDir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("Withdrawal journal directory must be private")
	}
	a.withdrawals = map[string]withdrawal{}
	files, err := filepath.Glob(filepath.Join(a.withdrawDir, "*.json"))
	if err != nil {
		return err
	}
	for _, path := range files {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return errors.New("Withdrawal journal file must be private")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var x withdrawal
		err = json.Unmarshal(data, &x)
		wipe(data)
		if err != nil || !taskIDPattern.MatchString(x.ID) || filepath.Base(path) != x.ID+".json" || !taskIDPattern.MatchString(x.AgentID) || !addressPattern.MatchString(x.Owner) || !addressPattern.MatchString(x.Wallet) || !strings.EqualFold(x.Owner, x.To) || x.Status != "pending" && x.Status != "confirmed" && x.Status != "failed" || !validWithdrawalTransaction(x) {
			return errors.New("Withdrawal journal is invalid")
		}
		a.withdrawals[x.ID] = x
	}
	return nil
}

func validWithdrawalTransaction(x withdrawal) bool {
	raw, err := hex.DecodeString(x.RawTx)
	if err != nil {
		return false
	}
	var tx types.Transaction
	if tx.UnmarshalBinary(raw) != nil || tx.ChainId().Cmp(big.NewInt(84532)) != 0 || !strings.EqualFold(tx.Hash().Hex(), x.Transaction) || tx.To() == nil || !strings.EqualFold(tx.To().Hex(), Asset) {
		return false
	}
	sender, err := types.Sender(types.LatestSignerForChainID(big.NewInt(84532)), &tx)
	if err != nil || !strings.EqualFold(sender.Hex(), x.Wallet) {
		return false
	}
	amount, err := Money(x.Amount)
	if err != nil || amount <= 0 {
		return false
	}
	expected := append(common.FromHex("0xa9059cbb"), common.LeftPadBytes(common.HexToAddress(x.To).Bytes(), 32)...)
	expected = append(expected, common.LeftPadBytes(big.NewInt(amount).Bytes(), 32)...)
	return bytes.Equal(tx.Data(), expected) && tx.Value().Sign() == 0
}

func (a *App) saveWithdrawal(x withdrawal) error {
	b, err := json.MarshalIndent(x, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(a.withdrawDir, ".withdraw-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(b); err != nil {
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
	if err = os.Rename(name, filepath.Join(a.withdrawDir, x.ID+".json")); err != nil {
		return err
	}
	if err = syncDirectory(a.withdrawDir); err != nil {
		return err
	}
	a.withdrawals[x.ID] = x
	return nil
}

func (a *App) refreshWithdrawal(ctx context.Context, chain withdrawalChain, x withdrawal) withdrawal {
	if x.Status != "pending" {
		return x
	}
	receipt, err := chain.TransactionReceipt(ctx, common.HexToHash(x.Transaction))
	if err != nil || receipt == nil || receipt.TxHash != common.HexToHash(x.Transaction) {
		return x
	}
	if receipt.Status == types.ReceiptStatusSuccessful {
		if !matchingUSDCTransfer(receipt, x) {
			return x
		}
		x.Status = "confirmed"
	} else {
		x.Status = "failed"
	}
	if a.saveWithdrawal(x) != nil {
		x.Status = "pending"
	}
	return x
}

func matchingUSDCTransfer(receipt *types.Receipt, x withdrawal) bool {
	amount, err := Money(x.Amount)
	if err != nil {
		return false
	}
	event := crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))
	from := common.BytesToHash(common.HexToAddress(x.Wallet).Bytes())
	to := common.BytesToHash(common.HexToAddress(x.To).Bytes())
	for _, log := range receipt.Logs {
		if log != nil && strings.EqualFold(log.Address.Hex(), Asset) && len(log.Topics) == 3 && log.Topics[0] == event && log.Topics[1] == from && log.Topics[2] == to && new(big.Int).SetBytes(log.Data).Cmp(big.NewInt(amount)) == 0 {
			return true
		}
	}
	return false
}

func (a *App) listWithdrawals(w http.ResponseWriter, r *http.Request) {
	agent, ok := a.ownedAgent(w, r)
	if !ok {
		return
	}
	a.withdrawMu.Lock()
	defer a.withdrawMu.Unlock()
	needsChain := false
	for _, x := range a.withdrawals {
		if x.AgentID == agent.ID && strings.EqualFold(x.Owner, agent.Owner) && x.Status == "pending" {
			needsChain = true
			break
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var chain withdrawalChain
	if needsChain {
		var err error
		chain, err = a.withdrawalClient(ctx)
		if err == nil {
			defer chain.Close()
		}
	}
	out := []withdrawalView{}
	for _, x := range a.withdrawals {
		if x.AgentID != agent.ID || !strings.EqualFold(x.Owner, agent.Owner) {
			continue
		}
		if chain != nil {
			x = a.refreshWithdrawal(ctx, chain, x)
		}
		out = append(out, x.view())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	jsonResponse(w, 200, out)
}

func (a *App) withdraw(w http.ResponseWriter, r *http.Request) {
	agent, ok := a.ownedAgent(w, r)
	if !ok {
		return
	}
	var input struct {
		ID       string `json:"id"`
		Amount   string `json:"amount"`
		Password string `json:"password"`
	}
	if !readSecretRequest(w, r, &input) {
		return
	}
	defer func() { input.Password = "" }()
	units, err := Money(input.Amount)
	if !taskIDPattern.MatchString(input.ID) || err != nil || units <= 0 {
		jsonResponse(w, 400, map[string]string{"error": "Enter a positive test USDC amount and valid request ID"})
		return
	}
	a.withdrawMu.Lock()
	defer a.withdrawMu.Unlock()
	if old, exists := a.withdrawals[input.ID]; exists {
		if old.AgentID != agent.ID || !strings.EqualFold(old.Owner, agent.Owner) || old.Amount != FormatMoney(units) {
			jsonResponse(w, 409, map[string]string{"error": "Withdrawal ID already used"})
			return
		}
		// Re-send exactly the same signed transaction; never create a second transfer.
		if old.Status == "pending" {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			if chain, e := a.withdrawalClient(ctx); e == nil {
				old = a.refreshWithdrawal(ctx, chain, old)
				if old.Status == "pending" {
					raw, e := hex.DecodeString(old.RawTx)
					if e == nil {
						var tx types.Transaction
						if tx.UnmarshalBinary(raw) == nil {
							_ = chain.SendTransaction(ctx, &tx)
						}
					}
				}
				chain.Close()
			}
		}
		jsonResponse(w, 200, old.view())
		return
	}
	for _, old := range a.withdrawals {
		if old.AgentID == agent.ID && old.Status == "pending" {
			jsonResponse(w, 409, map[string]string{"error": "Check the pending withdrawal before starting another"})
			return
		}
	}
	grant := a.grantFor(agent.ID, r)
	if grant == nil {
		jsonResponse(w, 423, map[string]string{"error": "Unlock this Agent wallet before withdrawing"})
		return
	}
	if input.Password == "" {
		jsonResponse(w, 400, map[string]string{"error": "Enter the wallet password to confirm withdrawal"})
		return
	}
	a.walletOps.Lock()
	defer a.walletOps.Unlock()
	key, err := decryptWallet(agent.KeyStore, input.Password)
	if err != nil || !strings.EqualFold(key.Address.Hex(), agent.Wallet) {
		if key != nil {
			wipeKey(key.PrivateKey)
		}
		jsonResponse(w, 400, map[string]string{"error": errWalletPassword.Error()})
		return
	}
	defer wipeKey(key.PrivateKey)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	chain, err := a.withdrawalClient(ctx)
	if err != nil {
		jsonResponse(w, 503, map[string]string{"error": "Could not connect to Base Sepolia"})
		return
	}
	defer chain.Close()
	chainID, err := chain.ChainID(ctx)
	if err != nil || (chainID == nil || chainID.Cmp(big.NewInt(84532)) != 0) {
		jsonResponse(w, 503, map[string]string{"error": "Withdrawal RPC is not Base Sepolia"})
		return
	}
	from, to, asset := common.HexToAddress(agent.Wallet), common.HexToAddress(agent.Owner), common.HexToAddress(Asset)
	balanceCall := append(common.FromHex("0x70a08231"), common.LeftPadBytes(from.Bytes(), 32)...)
	value, err := chain.CallContract(ctx, ethereum.CallMsg{To: &asset, Data: balanceCall}, nil)
	if err != nil || len(value) != 32 || new(big.Int).SetBytes(value).Cmp(big.NewInt(units)) < 0 {
		jsonResponse(w, 400, map[string]string{"error": "Not enough test USDC, or balance could not be checked"})
		return
	}
	data := append(common.FromHex("0xa9059cbb"), common.LeftPadBytes(to.Bytes(), 32)...)
	data = append(data, common.LeftPadBytes(big.NewInt(units).Bytes(), 32)...)
	msg := ethereum.CallMsg{From: from, To: &asset, Data: data}
	gas, err := chain.EstimateGas(ctx, msg)
	if err != nil || gas == 0 || gas > 150000 {
		jsonResponse(w, 400, map[string]string{"error": "Could not estimate the USDC transfer gas"})
		return
	}
	gas += gas / 5
	if gas > 150000 {
		gas = 150000
	}
	header, err := chain.HeaderByNumber(ctx, nil)
	if err != nil || header == nil || header.BaseFee == nil {
		jsonResponse(w, 503, map[string]string{"error": "Could not read the chain fee"})
		return
	}
	tip, err := chain.SuggestGasTipCap(ctx)
	if err != nil || tip == nil || tip.Sign() < 0 {
		jsonResponse(w, 503, map[string]string{"error": "Could not estimate the chain fee"})
		return
	}
	feeCap := new(big.Int).Add(new(big.Int).Mul(header.BaseFee, big.NewInt(2)), tip)
	ethBalance, err := chain.BalanceAt(ctx, from, nil)
	maxFee := new(big.Int).Mul(feeCap, new(big.Int).SetUint64(gas))
	if maxFee.Cmp(big.NewInt(10000000000000000)) > 0 {
		jsonResponse(w, 400, map[string]string{"error": "Estimated gas fee is too high"})
		return
	}
	if err != nil || ethBalance == nil || ethBalance.Cmp(maxFee) < 0 {
		jsonResponse(w, 400, map[string]string{"error": "Agent wallet needs Base Sepolia ETH for gas"})
		return
	}
	nonce, err := chain.PendingNonceAt(ctx, from)
	if err != nil {
		jsonResponse(w, 503, map[string]string{"error": "Could not read the wallet nonce"})
		return
	}
	tx := types.NewTx(&types.DynamicFeeTx{ChainID: chainID, Nonce: nonce, GasTipCap: tip, GasFeeCap: feeCap, Gas: gas, To: &asset, Value: big.NewInt(0), Data: data})
	grant.mu.Lock()
	if len(grant.wrapping) != 32 || grant.session != sessionToken(r) || !time.Now().Before(grant.expires) {
		grant.mu.Unlock()
		jsonResponse(w, 423, map[string]string{"error": "Wallet unlock expired; unlock again"})
		return
	}
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(chainID), key.PrivateKey)
	grant.mu.Unlock()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Could not sign withdrawal"})
		return
	}
	raw, err := signed.MarshalBinary()
	if err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Could not record withdrawal"})
		return
	}
	x := withdrawal{ID: input.ID, AgentID: agent.ID, Owner: agent.Owner, Wallet: agent.Wallet, To: agent.Owner, Amount: FormatMoney(units), Transaction: signed.Hash().Hex(), RawTx: hex.EncodeToString(raw), Status: "pending", Created: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := a.saveWithdrawal(x); err != nil {
		jsonResponse(w, 500, map[string]string{"error": "Could not save withdrawal; nothing was broadcast"})
		return
	}
	// A send error may mean the node accepted the transaction but lost its reply.
	// The saved hash/raw transaction can be checked or re-sent without a new signature.
	_ = chain.SendTransaction(ctx, signed)
	jsonResponse(w, 202, x.view())
}
