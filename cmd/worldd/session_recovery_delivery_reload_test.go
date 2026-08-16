package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

func TestHTTPRecoveryDeliveryAdapterRetireClearsCredentialAndRejectsNewDeliveries(t *testing.T) {
	attempts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	adapter := mustNewTestHTTPRecoveryDeliveryAdapter(t, server, "credential-0123456789", 1)
	adapter.logf = func(string, ...any) {}
	before := adapter.credentialSnapshot()
	if len(before) == 0 {
		t.Fatal("expected loaded relay credential before retirement")
	}
	clear(before)

	adapter.Retire()
	if got := adapter.credentialSnapshot(); len(got) != 0 {
		clear(got)
		t.Fatal("retired adapter retained relay credential")
	}

	err := adapter.Deliver(context.Background(), accountrecovery.Delivery{
		RequestID:   "retired-request",
		Destination: "alice@example.invalid",
		Proof:       []byte("retired-proof"),
		ExpiresAt:   time.Now().UTC().Add(time.Minute),
	})
	if !errors.Is(err, accountrecovery.ErrDeliveryPermanent) {
		t.Fatalf("retired delivery err=%v", err)
	}
	if attempts != 0 {
		t.Fatalf("retired adapter reached old relay attempts=%d", attempts)
	}
}
