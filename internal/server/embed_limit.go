package server

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	embedMaxTokens          = 512
	defaultAutoTokensPerSec = 16
	defaultCapacityPercent  = 50
	defaultEmbedBatch       = 4
	maxEmbedBatch           = 8
	maxTokensPerSecond      = 10000
	embedAttemptLimit       = 5
)

// embedSettings — скорость индексации. Хранится в app_settings.
// Режим auto берёт долю измеренной скорости модели. manual — фиксированные токены в секунду.
// Ноль в manual и paused останавливают воркер, не удаляя очередь.
type embedSettings struct {
	Mode            string `json:"mode"`
	TokensPerSecond int    `json:"embed_tokens_per_second"`
	CapacityPercent int    `json:"embed_capacity_percent"`
	BatchSize       int    `json:"embed_batch_size"`
	Paused          bool   `json:"paused"`
}

type embedEnv struct {
	TokensPerSecond int
	CapacityPercent int
	BatchSize       int
	Interval        time.Duration
	Concurrency     int
}

func defaultEmbedSettings(env embedEnv) embedSettings {
	batch := env.BatchSize
	if batch < 1 || batch > maxEmbedBatch {
		batch = defaultEmbedBatch
	}
	percent := env.CapacityPercent
	if percent < 1 || percent > 100 {
		percent = defaultCapacityPercent
	}
	rate := env.TokensPerSecond
	if rate < 0 || rate > maxTokensPerSecond {
		rate = 32
	}
	return embedSettings{
		Mode:            "auto",
		TokensPerSecond: rate,
		CapacityPercent: percent,
		BatchSize:       batch,
		Paused:          false,
	}
}

func validateEmbedSettings(in embedSettings) (embedSettings, error) {
	in.Mode = strings.TrimSpace(in.Mode)
	if in.Mode != "manual" && in.Mode != "auto" {
		return embedSettings{}, errRequest
	}
	if in.TokensPerSecond < 0 || in.TokensPerSecond > maxTokensPerSecond {
		return embedSettings{}, errRequest
	}
	if in.CapacityPercent < 1 || in.CapacityPercent > 100 {
		return embedSettings{}, errRequest
	}
	if in.BatchSize < 1 || in.BatchSize > maxEmbedBatch {
		return embedSettings{}, errRequest
	}
	return in, nil
}

// approxTokens — запасной счётчик, если TEI /tokenize недоступен.
// Четыре символа на токен, не больше окна модели (512).
func approxTokens(text string) int {
	n := (utf8.RuneCountInString(text) + 3) / 4
	if n < 1 {
		n = 1
	}
	if n > embedMaxTokens {
		n = embedMaxTokens
	}
	return n
}

func effectiveTokenRate(s embedSettings, measured float64) float64 {
	if s.Paused {
		return 0
	}
	if s.Mode == "manual" {
		return float64(s.TokensPerSecond)
	}
	if measured <= 0 {
		return defaultAutoTokensPerSec
	}
	return measured * float64(s.CapacityPercent) / 100
}

// nextCapacity — скользящее среднее токенов в секунду по пачкам.
func nextCapacity(prev float64, samples, tokens int, elapsed time.Duration) (float64, int) {
	if tokens <= 0 || elapsed <= 0 {
		return prev, samples
	}
	rate := float64(tokens) / elapsed.Seconds()
	if samples == 0 {
		return rate, 1
	}
	return prev*0.7 + rate*0.3, samples + 1
}

// tokenBucket ограничивает воркер. Время передаётся снаружи, чтобы тест не спал.
type tokenBucket struct {
	rate  float64
	have  float64
	burst float64
	at    time.Time
}

func (b *tokenBucket) setRate(rate float64, now time.Time) {
	b.rate = rate
	burst := rate * 2
	if burst < float64(embedMaxTokens) {
		burst = float64(embedMaxTokens)
	}
	if b.at.IsZero() {
		b.at = now
		b.burst = burst
		b.have = burst
		return
	}
	b.refill(now)
	b.burst = burst
	if b.have > b.burst {
		b.have = b.burst
	}
}

func (b *tokenBucket) refill(now time.Time) {
	if b.at.IsZero() {
		b.setRate(b.rate, now)
		return
	}
	dt := now.Sub(b.at).Seconds()
	if dt > 0 && b.rate > 0 {
		b.have += dt * b.rate
		if b.have > b.burst {
			b.have = b.burst
		}
	}
	b.at = now
}

// acquire списывает need токенов. Если их нет, возвращает ожидание и ничего не списывает.
func (b *tokenBucket) acquire(now time.Time, need float64) time.Duration {
	if need < 1 {
		need = 1
	}
	if b.rate <= 0 {
		return time.Minute
	}
	b.refill(now)
	if b.have >= need {
		b.have -= need
		return 0
	}
	missing := need - b.have
	return time.Duration(math.Ceil(missing / b.rate * float64(time.Second)))
}
