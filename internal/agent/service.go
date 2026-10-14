package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
)

const quickNodeURL = "https://x402.quicknode.com/base-mainnet"
const quickNodePayTo = "0xF46394adDdA95A3d5bCC1124605E3d15D204623C"
const baseMainnetUSDC = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
const quickNodePrice = "1000" // 0.001 Base Sepolia test USDC, pay-per-request.

var embeddedAddress = regexp.MustCompile(`0x[0-9a-fA-F]{40}`)

func requestedAddress(instruction string) (string, error) {
	lower := strings.ToLower(instruction)
	if !strings.Contains(lower, "base") || !strings.Contains(lower, "usdc") {
		return "", errors.New("Only Base mainnet USDC balance checks are supported")
	}
	matches := embeddedAddress.FindAllStringIndex(instruction, -1)
	if len(matches) != 1 {
		return "", errors.New("Enter exactly one Base mainnet wallet address (0x followed by 40 hex characters)")
	}
	start, end := matches[0][0], matches[0][1]
	if end < len(instruction) && isHex(instruction[end]) || start > 0 && isHex(instruction[start-1]) {
		return "", errors.New("Wallet address must be exactly 0x followed by 40 hex characters")
	}
	return instruction[start:end], nil
}

func isHex(b byte) bool { return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F' }

func quickNodeService(address string) Service {
	callData := "0x70a08231" + strings.Repeat("0", 24) + strings.ToLower(address[2:])
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "eth_call",
		"params": []any{map[string]string{"to": baseMainnetUSDC, "data": callData}, "latest"},
	})
	return Service{ID: "quicknode", Name: "QuickNode · Base mainnet USDC balance", URL: quickNodeURL, Method: http.MethodPost, Body: body, TargetAddress: address, PayTo: quickNodePayTo, Amount: quickNodePrice}
}

func serviceRequest(ctx context.Context, s Service) (*http.Request, error) {
	method := s.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, method, s.URL, bytes.NewReader(s.Body))
	if err != nil {
		return nil, err
	}
	if len(s.Body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

type balanceResult struct {
	WalletAddress string `json:"wallet_address"`
	Chain         string `json:"chain"`
	Token         string `json:"token"`
	BalanceUSDC   string `json:"balance_usdc"`
	BlockTag      string `json:"block_tag"`
}

func checkedServiceData(s Service, data []byte) (json.RawMessage, error) {
	if s.ID != "quicknode" {
		if !json.Valid(data) {
			return nil, errors.New("Service did not return JSON")
		}
		return json.RawMessage(data), nil
	}
	var response struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  string          `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &response) != nil || response.JSONRPC != "2.0" || string(response.ID) != "1" || len(response.Error) != 0 && string(response.Error) != "null" || len(response.Result) != 66 || !strings.HasPrefix(response.Result, "0x") {
		return nil, errors.New("QuickNode did not return a valid USDC balance")
	}
	for i := 2; i < len(response.Result); i++ {
		if !isHex(response.Result[i]) {
			return nil, errors.New("QuickNode balance is not hexadecimal")
		}
	}
	n, ok := new(big.Int).SetString(response.Result[2:], 16)
	if !ok {
		return nil, errors.New("QuickNode balance could not be read")
	}
	divisor := big.NewInt(1_000_000)
	whole, fraction := new(big.Int), new(big.Int)
	whole.QuoRem(n, divisor, fraction)
	result, _ := json.Marshal(balanceResult{WalletAddress: s.TargetAddress, Chain: "Base mainnet", Token: "USDC", BalanceUSDC: fmt.Sprintf("%s.%06d", whole.String(), fraction.Int64()), BlockTag: "latest"})
	return result, nil
}
