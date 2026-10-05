package agent

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	evm "github.com/x402-foundation/x402/go/v2/mechanisms/evm"
)

type smartPayment struct {
	chain           *smartChain
	record          agentRecord
	beforeBroadcast func(tx, digest string) error
}

// withKey never returns the private key. Lock waits only for local signing, not
// for RPC calls or transaction mining. Pending reservations cannot move USDC.
func (g *walletGrant) withKey(ctx context.Context, address string, fn func(*ecdsa.PrivateKey) error) error {
	return g.withKeyBefore(ctx, address, nil, fn)
}
func (g *walletGrant) withKeyBefore(ctx context.Context, address string, before func() error, fn func(*ecdsa.PrivateKey) error) error {
	if g == nil {
		return errWalletLocked
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if ctx.Err() != nil || len(g.wrapping) != 32 || !time.Now().Before(g.expires) {
		return errWalletLocked
	}
	if before != nil && before() != nil {
		return errors.New("Could not record payment authorization")
	}
	if ctx.Err() != nil || !time.Now().Before(g.expires) {
		return errWalletLocked
	}
	raw, err := openMemory(g.wrapping, g.wallet)
	if err != nil {
		return errWalletLocked
	}
	defer wipe(raw)
	pk, err := crypto.ToECDSA(raw)
	if err != nil {
		return errWalletLocked
	}
	defer wipeKey(pk)
	if !strings.EqualFold(crypto.PubkeyToAddress(pk.PublicKey).Hex(), address) {
		return errWalletLocked
	}
	return fn(pk)
}
func (s *walletSigner) signSmart(ctx context.Context, domain evm.TypedDataDomain, fields map[string][]evm.TypedDataField, primary string, message map[string]interface{}) ([]byte, error) {
	p := s.smart
	if p == nil || s.grant == nil || !s.grant.valid(s.grant.session) {
		return nil, errWalletLocked
	}
	if primary != "TransferWithAuthorization" || domain.Name != "USDC" || domain.Version != "2" || domain.ChainID == nil || domain.ChainID.Cmp(big.NewInt(84532)) != 0 || !strings.EqualFold(domain.VerifyingContract, Asset) {
		return nil, errors.New("Only Base Sepolia USDC payment signatures are allowed")
	}
	from, ok1 := message["from"].(string)
	to, ok2 := message["to"].(string)
	value, ok3 := message["value"].(*big.Int)
	after, ok4 := message["validAfter"].(*big.Int)
	before, ok5 := message["validBefore"].(*big.Int)
	nonce, ok6 := message["nonce"].([]byte)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || value == nil || after == nil || before == nil || len(nonce) != 32 || !addressPattern.MatchString(to) || !strings.EqualFold(from, p.record.Wallet) || !strings.EqualFold(s.address, p.record.Wallet) || value.Sign() <= 0 || value.Cmp(big.NewInt(MaxDemoAtomic)) > 0 || after.Sign() < 0 || before.Sign() <= 0 {
		return nil, errors.New("Invalid smart wallet payment")
	}
	if err := p.chain.verify(ctx, p.record); err != nil {
		return nil, err
	}
	address := common.HexToAddress(p.record.Wallet)
	var nonce32 [32]byte
	copy(nonce32[:], nonce)
	args := []any{common.HexToAddress(to), value, after, before, nonce32}
	digest, err := evm.HashTypedData(domain, fields, primary, message)
	if err != nil {
		return nil, errors.New("Invalid payment typed data")
	}
	expected, err := p.chain.call(ctx, address, "paymentDigest", args...)
	if err != nil {
		return nil, err
	}
	expectedHash := expected[0].([32]byte)
	if !bytes.Equal(digest, expectedHash[:]) {
		return nil, errors.New("Payment fields do not match the wallet contract")
	}
	s.grant.mu.Lock()
	if s.used || len(s.grant.wrapping) != 32 || !time.Now().Before(s.grant.expires) {
		s.grant.mu.Unlock()
		return nil, errWalletLocked
	}
	if s.beforeSign == nil {
		s.grant.mu.Unlock()
		return nil, errors.New("Could not record payment attempt")
	}
	s.used = true
	s.grant.mu.Unlock()
	data, err := walletABI.Pack("reservePayment", args...)
	if err != nil {
		return nil, errors.New("Invalid reservation")
	}
	if err = p.reserve(ctx, s.grant, address, data, common.BytesToHash(digest)); err != nil {
		return nil, err
	}
	var signature []byte
	err = s.grant.withKeyBefore(ctx, p.record.SessionAddress, s.beforeSign, func(pk *ecdsa.PrivateKey) error {
		raw, e := crypto.Sign(digest, pk)
		if e != nil {
			return errors.New("Could not sign payment")
		}
		raw[64] += 27
		signature = append([]byte{1}, raw...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	valid, err := p.chain.call(ctx, address, "isValidSignature", expectedHash, signature)
	if err != nil || valid[0].([4]byte) != ([4]byte{0x16, 0x26, 0xba, 0x7e}) {
		return nil, errors.New("Onchain authorization is revoked, expired, or unavailable")
	}
	if !s.grant.valid(s.grant.session) {
		return nil, errWalletLocked
	}
	return signature, nil
}
func (p *smartPayment) reserve(ctx context.Context, g *walletGrant, to common.Address, data []byte, digest common.Hash) error {
	from := common.HexToAddress(p.record.SessionAddress)
	rpc := p.chain.rpc
	nonce, err := rpc.PendingNonceAt(ctx, from)
	if err != nil {
		return errors.New("Could not read session transaction nonce")
	}
	price, err := rpc.SuggestGasPrice(ctx)
	if err != nil || price.Sign() <= 0 || price.Cmp(big.NewInt(4000000000)) > 0 {
		return errors.New("Reservation gas price unavailable or above the 4 gwei cap")
	}
	gas, err := rpc.EstimateGas(ctx, ethereum.CallMsg{From: from, To: &to, Data: data, GasPrice: price})
	if err != nil {
		return errors.New("Reservation refused: check authorization, whitelist, limits, expiry, and session ETH")
	}
	gas += gas / 5
	if gas > 500000 {
		return errors.New("Reservation exceeds the 500000 gas cap")
	}
	tx := types.NewTx(&types.LegacyTx{Nonce: nonce, To: &to, Value: big.NewInt(0), Gas: gas, GasPrice: price, Data: data})
	var signed *types.Transaction
	err = g.withKey(ctx, p.record.SessionAddress, func(pk *ecdsa.PrivateKey) error {
		var e error
		signed, e = types.SignTx(tx, types.LatestSignerForChainID(big.NewInt(84532)), pk)
		return e
	})
	if err != nil {
		return err
	}
	if p.beforeBroadcast == nil || p.beforeBroadcast(signed.Hash().Hex(), digest.Hex()) != nil {
		return errors.New("Could not save reservation transaction; nothing was sent")
	}
	if !g.valid(g.session) {
		return errWalletLocked
	}
	if rpc.SendTransaction(ctx, signed) != nil {
		return errors.New("Reservation submission is unconfirmed; check the saved transaction hash")
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		receipt, e := rpc.TransactionReceipt(ctx, signed.Hash())
		if e == nil {
			if receipt.Status != types.ReceiptStatusSuccessful {
				return errors.New("Onchain reservation failed; no payment signature released")
			}
			return nil
		}
		if !errors.Is(e, ethereum.NotFound) {
			return errors.New("Could not confirm reservation; no payment signature released")
		}
		select {
		case <-ctx.Done():
			return errors.New("Reservation is pending; no payment signature released")
		case <-ticker.C:
		}
	}
}
