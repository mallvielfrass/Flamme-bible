// Пакет secret шифрует токен провайдера. Ключ берётся только из AI_SECRET_KEY.
// Открытый токен не пишется в журнал: наружу уходит Scrub и последние 4 символа.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ErrDisabled — ключ не задан, ИИ выключен.
var ErrDisabled = errors.New("ИИ не настроен")

const maxErrorRunes = 240

var bearerRE = regexp.MustCompile(`(?i)bearer\s+\S+`)

// LoadKey читает 32 байта ключа AES из AI_SECRET_KEY (base64).
// Пустая переменная — ErrDisabled. Неверная длина или кодировка — другая ошибка, без значения ключа.
func LoadKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("AI_SECRET_KEY"))
	if raw == "" {
		return nil, ErrDisabled
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, errors.New("AI_SECRET_KEY должен быть 32 байта в base64")
	}
	return key, nil
}

// Seal шифрует plaintext AES-256-GCM. nonce — 12 случайных байт, его хранят рядом с ciphertext.
// aad связывается с пользователем и провайдером (user_id|provider_id) и в шифртекст не входит.
func Seal(key, plaintext []byte, aad string) (ciphertext, nonce []byte, err error) {
	gcm, err := gcmFrom(key)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ciphertext = gcm.Seal(nil, nonce, plaintext, []byte(aad))
	return ciphertext, nonce, nil
}

// Open расшифровывает ciphertext. Чужой ключ, nonce или aad дают ошибку.
func Open(key, ciphertext, nonce []byte, aad string) ([]byte, error) {
	gcm, err := gcmFrom(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("nonce")
	}
	return gcm.Open(nil, nonce, ciphertext, []byte(aad))
}

func gcmFrom(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("ключ")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Last4 — единственная часть токена, которую можно показать.
func Last4(token string) string {
	r := []rune(token)
	if len(r) <= 4 {
		return token
	}
	return string(r[len(r)-4:])
}

// Scrub убирает сам токен и куски «Bearer ...», затем обрезает текст ошибки провайдера.
func Scrub(text, token string) string {
	if token != "" && len(token) >= 8 {
		text = strings.ReplaceAll(text, token, "***")
	}
	text = bearerRE.ReplaceAllString(text, "Bearer ***")
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= maxErrorRunes {
		return text
	}
	return string([]rune(text)[:maxErrorRunes])
}
