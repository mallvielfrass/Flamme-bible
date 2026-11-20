package main

import (
	"os"
	"strings"
	"testing"

	"golang.org/x/term"
)

func TestLoginPasswordRequiredWhenNotTerminal(t *testing.T) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("stdin — терминал")
	}
	err := run([]string{"login", "--server", "https://example.com", "--email", "a@example.com"})
	if err == nil || !strings.Contains(err.Error(), "--password") {
		t.Fatalf("ожидался отказ без --password, получено: %v", err)
	}
}
