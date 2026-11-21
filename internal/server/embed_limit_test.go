package server

import (
	"math"
	"testing"
	"time"
	"unicode/utf8"
)

func TestApproxTokens(t *testing.T) {
	if approxTokens("") != 1 {
		t.Fatal("пустой текст")
	}
	text := "привет"
	want := (utf8.RuneCountInString(text) + 3) / 4
	if approxTokens(text) != want {
		t.Fatalf("оценка %d", approxTokens(text))
	}
	long := string(make([]rune, 5000))
	if approxTokens(long) != embedMaxTokens {
		t.Fatalf("потолок %d", approxTokens(long))
	}
}

func TestEffectiveTokenRate(t *testing.T) {
	auto := embedSettings{Mode: "auto", CapacityPercent: 50}
	if effectiveTokenRate(auto, 0) != defaultAutoTokensPerSec {
		t.Fatal("до измерения берётся запасная скорость")
	}
	if effectiveTokenRate(auto, 80) != 40 {
		t.Fatalf("половина мощности %v", effectiveTokenRate(auto, 80))
	}
	manual := embedSettings{Mode: "manual", TokensPerSecond: 12}
	if effectiveTokenRate(manual, 80) != 12 {
		t.Fatal("ручной режим")
	}
	manual.Paused = true
	if effectiveTokenRate(manual, 80) != 0 {
		t.Fatal("пауза")
	}
	manual.Paused = false
	manual.TokensPerSecond = 0
	if effectiveTokenRate(manual, 80) != 0 {
		t.Fatal("ноль токенов в секунду останавливает")
	}
}

func TestTokenBucket(t *testing.T) {
	var bucket tokenBucket
	now := time.Now()
	bucket.setRate(10, now)
	bucket.burst = 10
	bucket.have = 10
	if wait := bucket.acquire(now, 10); wait != 0 {
		t.Fatalf("первый запас %s", wait)
	}
	wait := bucket.acquire(now, 10)
	if wait < time.Second || wait > 2*time.Second {
		t.Fatalf("ожидание %s", wait)
	}
	later := now.Add(wait)
	if wait := bucket.acquire(later, 10); wait != 0 {
		t.Fatalf("после ожидания %s", wait)
	}
}

func TestCapacityAverage(t *testing.T) {
	rate, n := nextCapacity(0, 0, 100, time.Second)
	if n != 1 || rate != 100 {
		t.Fatalf("первый замер %v %d", rate, n)
	}
	rate, n = nextCapacity(rate, n, 40, time.Second)
	if n != 2 || math.Abs(rate-82) > 0.01 {
		t.Fatalf("среднее %v", rate)
	}
	rate, n = nextCapacity(rate, n, 0, time.Second)
	if n != 2 {
		t.Fatal("пустая пачка не считается")
	}
}

func TestValidateEmbedSettings(t *testing.T) {
	ok, err := validateEmbedSettings(embedSettings{Mode: "auto", TokensPerSecond: 0, CapacityPercent: 50, BatchSize: 4})
	if err != nil || ok.Mode != "auto" {
		t.Fatal(err)
	}
	if _, err := validateEmbedSettings(embedSettings{Mode: "fast", CapacityPercent: 50, BatchSize: 4}); err == nil {
		t.Fatal("режим")
	}
	if _, err := validateEmbedSettings(embedSettings{Mode: "manual", TokensPerSecond: -1, CapacityPercent: 50, BatchSize: 4}); err == nil {
		t.Fatal("скорость")
	}
	if _, err := validateEmbedSettings(embedSettings{Mode: "manual", TokensPerSecond: maxTokensPerSecond + 1, CapacityPercent: 50, BatchSize: 4}); err == nil {
		t.Fatal("потолок")
	}
	if _, err := validateEmbedSettings(embedSettings{Mode: "auto", CapacityPercent: 0, BatchSize: 4}); err == nil {
		t.Fatal("процент")
	}
	if _, err := validateEmbedSettings(embedSettings{Mode: "auto", CapacityPercent: 50, BatchSize: 9}); err == nil {
		t.Fatal("пачка")
	}
}

func TestParseTokenCounts(t *testing.T) {
	counts, err := parseTokenCounts([]byte(`[[1,2,3],[4]]`), 2)
	if err != nil || counts[0] != 3 || counts[1] != 1 {
		t.Fatalf("%v %v", counts, err)
	}
	counts, err = parseTokenCounts([]byte(`[[{"id":1,"text":"а"},{"id":2,"text":"б"}],[{"id":3,"text":"в"}]]`), 2)
	if err != nil || counts[0] != 2 || counts[1] != 1 {
		t.Fatalf("объекты %v %v", counts, err)
	}
	info, err := parseEmbedInfo([]byte(`{"model_id":"cointegrated/rubert-tiny2","embedding_dimension":312,"max_input_length":512}`))
	if err != nil || info.ModelID != "cointegrated/rubert-tiny2" || info.Dim != 312 || info.MaxTokens != 512 {
		t.Fatalf("%+v %v", info, err)
	}
}
