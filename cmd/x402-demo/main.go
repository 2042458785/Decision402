package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"Decision402/internal/probe"

	x402 "github.com/x402-foundation/x402/go/v2"
	x402http "github.com/x402-foundation/x402/go/v2/http"
	nethttpmw "github.com/x402-foundation/x402/go/v2/http/nethttp"
	buyerEVM "github.com/x402-foundation/x402/go/v2/mechanisms/evm/exact/client"
	sellerEVM "github.com/x402-foundation/x402/go/v2/mechanisms/evm/exact/server"
	evmsigners "github.com/x402-foundation/x402/go/v2/signers/evm"
)

const (
	baseSepolia     = "eip155:84532"
	baseSepoliaUSDC = "0x036CbD53842c5426634e7929541eC2318f3dCF7e"
	priceUSD        = "$0.001"
	maxAmountAtomic = "1000" // 0.001 USDC at 6 decimals
	facilitatorURL  = "https://x402.org/facilitator"
)

var evmAddress = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

func main() {
	mode := flag.String("mode", "inspect", "inspect, serve, or pay")
	payTo := flag.String("pay-to", "", "seller wallet for serve; expected seller wallet for pay")
	listen := flag.String("listen", "127.0.0.1:4021", "listen address for serve")
	resource := flag.String("url", "http://127.0.0.1:4021/data", "resource URL for inspect or pay")
	keyFile := flag.String("key-file", ".buyer-key", "file containing a test-only EVM private key for pay")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("unexpected positional arguments")
	}
	var err error
	switch *mode {
	case "serve":
		err = serve(*listen, *payTo) //服务器端
	case "inspect":
		err = request(*resource, "", "") //只查看报价
	case "pay":
		err = request(*resource, *payTo, *keyFile) //支付
	default:
		err = errors.New("-mode must be inspect, serve, or pay")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func serve(listen, payTo string) error {
	if !evmAddress.MatchString(payTo) {
		return errors.New("serve requires -pay-to with a 0x + 40 hex Base Sepolia receiving address")
	}
	if _, _, err := net.SplitHostPort(listen); err != nil {
		return fmt.Errorf("invalid -listen address: %w", err)
	}
	facilitator := x402http.NewHTTPFacilitatorClient(&x402http.FacilitatorConfig{URL: facilitatorURL})
	routes := x402http.RoutesConfig{
		"GET /data": {
			Accepts: x402http.PaymentOptions{{
				Scheme: "exact", Price: priceUSD, Network: baseSepolia, PayTo: payTo,
			}},
			Description: "Decision402 sample dataset (static demo data)",
			MimeType:    "application/json",
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"ok","payment_required_on":"/data"}`)
	})
	mux.HandleFunc("GET /data", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"dataset": "Decision402 demo weather data", "city": "Tokyo",
			"temperature_c": 23, "sample_only": true,
		})
	})
	handler := nethttpmw.X402Payment(nethttpmw.Config{
		Routes: routes, Facilitator: facilitator,
		Schemes: []nethttpmw.SchemeConfig{{Network: x402.Network(baseSepolia), Server: sellerEVM.NewExactEvmScheme()}},
		Timeout: 30 * time.Second,
	})(mux)
	log.Printf("x402 demo listening on %s; Base Sepolia USDC %s to %s", listen, priceUSD, payTo)
	return (&http.Server{Addr: listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second}).ListenAndServe()
}

func request(rawURL, expectedPayTo, keyFile string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "http" || u.User != nil || !isLoopback(u.Hostname()) || u.Path != "/data" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("this demo accepts only a local http://localhost-or-127.0.0.1:<port>/data URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	baseClient := &http.Client{
		Timeout:       40 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	httpClient := baseClient
	var getSettlement func(http.Header) (*x402.SettleResponse, error)
	if keyFile != "" {
		if !evmAddress.MatchString(expectedPayTo) {
			return errors.New("pay requires -pay-to with the expected seller address")
		}
		key, err := readTestKey(keyFile)
		if err != nil {
			return err
		}
		signer, err := evmsigners.NewClientSignerFromPrivateKey(key)
		if err != nil {
			return errors.New("test wallet key is invalid")
		}
		client := x402.Newx402Client(x402.WithSpendControls(x402.SpendControls{MaxAmountPerPayment: priceUSD}))
		cfg, err := probe.LoadConfig(".env")
		if err != nil {
			return fmt.Errorf("Intercepta configuration: %w", err)
		}
		client.OnBeforePaymentCreation(paymentGate(probe.NewClient(cfg.APIKey, 15*time.Second), expectedPayTo))
		client.Register(x402.Network(baseSepolia), buyerEVM.NewExactEvmScheme(signer, nil))
		protocolClient := x402http.Newx402HTTPClient(client)
		getSettlement = func(h http.Header) (*x402.SettleResponse, error) {
			return protocolClient.GetPaymentSettleResponse(map[string]string{
				"PAYMENT-RESPONSE": h.Get("PAYMENT-RESPONSE"),
			})
		}
		httpClient = x402http.WrapHTTPClientWithPayment(baseClient, protocolClient)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024+1))
	if err != nil {
		return err
	}
	if len(body) > 64*1024 {
		return errors.New("response exceeded 64 KiB")
	}
	fmt.Printf("HTTP %d\n", resp.StatusCode)
	if header := resp.Header.Get("PAYMENT-REQUIRED"); header != "" {
		if decoded, err := base64.StdEncoding.DecodeString(header); err == nil && json.Valid(decoded) {
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, decoded, "", "  "); err == nil {
				fmt.Printf("x402 offer:\n%s\n", pretty.String())
			}
		}
	}
	if len(body) != 0 {
		fmt.Printf("response body:\n%s\n", body)
	}
	if getSettlement != nil && resp.StatusCode == http.StatusOK {
		settlement, err := getSettlement(resp.Header)
		if err != nil || settlement == nil || !settlement.Success {
			return errors.New("HTTP 200 was received, but a successful x402 settlement receipt was not confirmed")
		}
		fmt.Printf("x402 settlement: success=%t network=%s tx=%s\n", settlement.Success, settlement.Network, settlement.Transaction)
	}
	if keyFile == "" && resp.StatusCode != http.StatusPaymentRequired {
		return fmt.Errorf("inspection expected HTTP 402 but received %d", resp.StatusCode)
	}
	if keyFile != "" && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("testnet payment did not complete: HTTP %d", resp.StatusCode)
	}
	return nil
}

func checkOffer(requirements x402.PaymentRequirementsView, expectedPayTo string) error {
	if requirements == nil {
		return errors.New("missing x402 payment requirements")
	}
	if requirements.GetScheme() != "exact" || requirements.GetNetwork() != baseSepolia {
		return errors.New("payment must use exact on Base Sepolia")
	}
	if !strings.EqualFold(requirements.GetAsset(), baseSepoliaUSDC) {
		return errors.New("payment asset differs from Base Sepolia USDC")
	}
	if !strings.EqualFold(requirements.GetPayTo(), expectedPayTo) {
		return errors.New("payTo differs from the expected seller")
	}
	amount, ok := new(big.Int).SetString(requirements.GetAmount(), 10)
	max, _ := new(big.Int).SetString(maxAmountAtomic, 10)
	if !ok || amount.Sign() <= 0 || amount.Cmp(max) > 0 {
		return errors.New("payment amount is invalid or exceeds 0.001 USDC")
	}
	if method, ok := requirements.GetExtra()["assetTransferMethod"].(string); ok && method != "" && method != "eip3009" {
		return errors.New("only EIP-3009 authorization is supported by this demo")
	}
	return nil
}

func readTestKey(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", errors.New("test wallet key file is missing; create .buyer-key locally")
	}
	if info.Mode().Perm()&0077 != 0 {
		return "", errors.New("test wallet key file must be owner-only (chmod 600 .buyer-key)")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("could not read the test wallet key file")
	}
	return strings.TrimSpace(string(b)), nil
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1"
}
