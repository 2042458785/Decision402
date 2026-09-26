package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"Decision402/internal/probe"
	x402 "github.com/x402-foundation/x402/go/v2"
)

type riskScanner interface {
	Screen(context.Context, string) probe.Decision
}

// paymentGate runs inside the SDK before authorization creation/signing.
func paymentGate(scanner riskScanner, expectedPayTo string) x402.BeforePaymentCreationHook {
	return func(ctx x402.PaymentCreationContext) (*x402.BeforePaymentCreationHookResult, error) {
		if err := checkOffer(ctx.SelectedRequirements, expectedPayTo); err != nil {
			log.Printf("decision=deny stage=quote reason=%q", err.Error())
			return &x402.BeforePaymentCreationHookResult{Abort: true, Reason: err.Error()}, nil
		}
		payTo := ctx.SelectedRequirements.GetPayTo()
		decision := scanner.Screen(ctx.Ctx, payTo)
		if !strings.EqualFold(decision.Address, payTo) {
			decision.Action, decision.Reason = "hold", "scan address does not match the selected payTo"
		}
		if ctx.Ctx.Err() != nil {
			decision.Action, decision.Reason = "hold", "request expired before signing"
		}
		evidence, _ := json.Marshal(decision)
		log.Printf("payment_network=%s pre_sign_decision=%s", baseSepolia, evidence)
		if decision.Action != "allow" {
			return &x402.BeforePaymentCreationHookResult{Abort: true, Reason: decision.Action + ": " + decision.Reason}, nil
		}
		return nil, nil
	}
}
