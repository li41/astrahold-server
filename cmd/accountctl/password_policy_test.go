package main

import (
	"strings"
	"testing"
)

func TestPasswordMinimumBoundary(t *testing.T) {
	password, err := readPassword(strings.NewReader("123456\n"))
	if err != nil {
		t.Fatalf("six-byte password must be accepted: %v", err)
	}
	defer clear(password)
	if string(password) != "123456" {
		t.Fatalf("password=%q", password)
	}

	if password, err := readPassword(strings.NewReader("12345\n")); err == nil {
		clear(password)
		t.Fatal("five-byte password must be rejected")
	}
}
