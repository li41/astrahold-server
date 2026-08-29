package main

import "testing"

func TestSessionRecoveryPasswordMinimumBoundary(t *testing.T) {
	if !validSessionRecoveryPassword([]byte("123456")) {
		t.Fatal("six-byte recovery password must be accepted")
	}
	if validSessionRecoveryPassword([]byte("12345")) {
		t.Fatal("five-byte recovery password must be rejected")
	}
	if validSessionRecoveryPassword([]byte("123456\n")) {
		t.Fatal("recovery password containing a newline must be rejected")
	}
}
