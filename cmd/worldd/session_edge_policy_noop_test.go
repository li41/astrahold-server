package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionEdgePolicyReloadNoopIgnoresRepresentationOnlyChanges(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "edge-ca.pem")
	policyPath := filepath.Join(dir, "edge-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "edge-ca-a", now)
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-001",
		ForwardedHeader: "x-forwarded-for",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{
			{
				Prefixes: []string{"127.0.0.2/32", "127.0.0.3/32"},
				DNSNames: []string{"edge-a.astrahold.test", "edge-alt.astrahold.test"},
			},
			{
				Prefixes: []string{"10.0.0.0/8"},
				DNSNames: []string{"internal.astrahold.test"},
			},
		},
	})

	runtime, err := newReloadableSessionEdgePolicy(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	// Same effective authority, but revision, binding order/grouping, prefix
	// spelling, DNS case/order, and duplicate identities all differ.
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-label-only",
		ForwardedHeader: "X-Forwarded-For",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{
			{
				Prefixes: []string{"10.0.0.0/8"},
				DNSNames: []string{"INTERNAL.ASTRAHOLD.TEST"},
			},
			{
				Prefixes: []string{"127.0.0.3"},
				DNSNames: []string{"EDGE-ALT.ASTRAHOLD.TEST", "edge-a.astrahold.test"},
			},
			{
				Prefixes: []string{"127.0.0.2"},
				DNSNames: []string{"edge-a.astrahold.test", "EDGE-ALT.ASTRAHOLD.TEST", "edge-a.astrahold.test"},
			},
		},
	})

	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if result.AuthorityChanged {
		t.Fatalf("representation-only reload unexpectedly changed authority: %+v", result)
	}
	if result.PreviousGeneration != 1 || result.Generation != 1 || result.PreviousRevision != "edge-001" || result.Revision != "edge-001" {
		t.Fatalf("no-op reload advanced or replaced current state: %+v", result)
	}
	metadata := runtime.Snapshot()
	if metadata.Generation != 1 || metadata.Revision != "edge-001" || metadata.HeaderMode != "x-forwarded-for" {
		t.Fatalf("current snapshot changed on semantic no-op: %+v", metadata)
	}

	// A real authority change must still publish the next generation.
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-002",
		ForwardedHeader: "forwarded",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{
			{
				Prefixes: []string{"127.0.0.2/32", "127.0.0.3/32"},
				DNSNames: []string{"edge-a.astrahold.test", "edge-alt.astrahold.test"},
			},
			{
				Prefixes: []string{"10.0.0.0/8"},
				DNSNames: []string{"internal.astrahold.test"},
			},
		},
	})
	changed, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if !changed.AuthorityChanged || changed.PreviousGeneration != 1 || changed.Generation != 2 || changed.Revision != "edge-002" || changed.HeaderMode != "forwarded" {
		t.Fatalf("real authority change did not publish: %+v", changed)
	}
}

func TestSessionEdgePolicyReloadNoopIgnoresDuplicateCARootPEM(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "edge-ca.pem")
	policyPath := filepath.Join(dir, "edge-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "edge-ca-a", now)
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-001",
		ForwardedHeader: "x-forwarded-for",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{{
			Prefixes: []string{"127.0.0.2/32"},
			DNSNames: []string{"edge-a.astrahold.test"},
		}},
	})
	runtime, err := newReloadableSessionEdgePolicy(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	pemData, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	duplicated := append(append([]byte{}, pemData...), pemData...)
	if err := os.WriteFile(caPath, duplicated, 0o600); err != nil {
		t.Fatal(err)
	}
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-ca-reformatted",
		ForwardedHeader: "x-forwarded-for",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{{
			Prefixes: []string{"127.0.0.2/32"},
			DNSNames: []string{"edge-a.astrahold.test"},
		}},
	})

	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if result.AuthorityChanged || result.Generation != 1 || result.Revision != "edge-001" || result.RootCount != 1 {
		t.Fatalf("duplicate PEM representation changed current authority: %+v", result)
	}
}

func TestSessionEdgePolicyReloadDetectsDifferentCARootCertificate(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "edge-ca.pem")
	policyPath := filepath.Join(dir, "edge-policy.json")
	writeSessionProxyTestCA(t, caPath, 1, "edge-ca", now)
	writeSessionEdgePolicyTest(t, policyPath, sessionEdgePolicyDefinition{
		SchemaVersion:   sessionEdgePolicySchemaVersion,
		Revision:        "edge-001",
		ForwardedHeader: "x-forwarded-for",
		ClientCAFile:    filepath.Base(caPath),
		Bindings: []sessionEdgePolicyBindingDefinition{{
			Prefixes: []string{"127.0.0.2/32"},
			DNSNames: []string{"edge-a.astrahold.test"},
		}},
	})
	runtime, err := newReloadableSessionEdgePolicy(policyPath, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	// Same path, subject, serial, policy text, and root count; only the actual
	// certificate/key material changes. Raw DER therefore changes authority.
	writeSessionProxyTestCA(t, caPath, 1, "edge-ca", now)
	result, err := runtime.Reload()
	if err != nil {
		t.Fatal(err)
	}
	if !result.AuthorityChanged || result.PreviousGeneration != 1 || result.Generation != 2 {
		t.Fatalf("different CA certificate did not advance authority: %+v", result)
	}
}
