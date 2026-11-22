package main

import (
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestParseReleaseAddRequiresVersionAndFile(t *testing.T) {
	_, err := parseReleaseAdd(nil)
	if err == nil || !strings.Contains(err.Error(), "--version") {
		t.Fatalf("ошибка %v", err)
	}
	_, err = parseReleaseAdd([]string{"-version", "1.2.3"})
	if err == nil || !strings.Contains(err.Error(), "--linux") {
		t.Fatalf("ошибка %v", err)
	}
}

func TestParseReleaseAddHelp(t *testing.T) {
	_, err := parseReleaseAdd([]string{"-h"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("ошибка %v", err)
	}
}

func TestParseReleaseAddFlags(t *testing.T) {
	flags, err := parseReleaseAdd([]string{
		"-version", "1.2.3",
		"-linux", "./linux.bin",
		"-windows", "./win.exe",
		"-notes", "  что нового  ",
		"-linux-sha256", "abc",
		"-publish",
	})
	if err != nil {
		t.Fatal(err)
	}
	if flags.version != "1.2.3" || flags.linux != "./linux.bin" || flags.windows != "./win.exe" {
		t.Fatalf("файлы %+v", flags)
	}
	if flags.notes != "что нового" || flags.linuxSHA != "abc" || !flags.publish {
		t.Fatalf("флаги %+v", flags)
	}
}

func TestRunReleaseUsage(t *testing.T) {
	if code := runRelease(nil); code != 2 {
		t.Fatalf("без команды код %d", code)
	}
	if code := runRelease([]string{"unknown"}); code != 2 {
		t.Fatalf("неизвестная код %d", code)
	}
	if code := runRelease([]string{"help"}); code != 0 {
		t.Fatalf("help код %d", code)
	}
}
