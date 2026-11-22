// Пакет netguard проверяет base_url провайдера и даёт http.Client,
// который не ходит в частные сети. На сохранении адреса смотрим все ответы DNS.
// При каждом соединении Control проверяет уже выбранный IP: так не проходит подмена DNS.
package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	errBadURL = errors.New("адрес провайдера")
	errDenied = errors.New("адрес запрещён")

	prefixThis  = netip.MustParsePrefix("0.0.0.0/8")
	prefixCGNAT = netip.MustParsePrefix("100.64.0.0/10")
	prefixIETF  = netip.MustParsePrefix("192.0.0.0/24")
	prefixBench = netip.MustParsePrefix("198.18.0.0/15")
	prefixHigh  = netip.MustParsePrefix("224.0.0.0/3")

	ipv4Literal = regexp.MustCompile(`^(0[xX][0-9A-Fa-f]+|[0-9]+)(\.(0[xX][0-9A-Fa-f]+|[0-9]+)){0,3}$`)
)

// Resolver возвращает все адреса имени. В проверках его подменяют.
type Resolver func(ctx context.Context, host string) ([]netip.Addr, error)

// ValidateBaseURL разрешает только https без логина и пароля в адресе.
// Порт любой. Имя должно разрешаться лишь в публичные адреса: если среди них есть запрещённый, адрес отклонён.
func ValidateBaseURL(ctx context.Context, raw string) (string, error) {
	return validateBaseURL(ctx, raw, defaultResolve)
}

// ValidateBaseURLWith делает то же, но DNS задаёт вызывающий код.
func ValidateBaseURLWith(ctx context.Context, raw string, resolve Resolver) (string, error) {
	if resolve == nil {
		resolve = defaultResolve
	}
	return validateBaseURL(ctx, raw, resolve)
}

func defaultResolve(ctx context.Context, host string) ([]netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func validateBaseURL(ctx context.Context, raw string, resolve Resolver) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n\\") {
		return "", errBadURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errBadURL
	}
	host := u.Hostname()
	if host == "" {
		return "", errBadURL
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errBadURL
		}
	}
	literal, isIP, bad := classifyHost(host)
	if bad {
		return "", errBadURL
	}
	if isIP {
		if Forbidden(literal) {
			return "", errBadURL
		}
		host = literal.String()
	} else {
		name := strings.ToLower(strings.TrimSuffix(host, "."))
		if name == "" || name == "localhost" {
			return "", errBadURL
		}
		addrs, err := resolve(ctx, name)
		if err != nil || len(addrs) == 0 {
			return "", errBadURL
		}
		for _, addr := range addrs {
			if Forbidden(addr) {
				return "", errBadURL
			}
		}
		host = name
	}
	host = joinHostPort(host, port)
	path := strings.TrimRight(u.EscapedPath(), "/")
	if path == "/" {
		path = ""
	}
	return "https://" + host + path, nil
}

func joinHostPort(host, port string) string {
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

func classifyHost(host string) (netip.Addr, bool, bool) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap(), true, false
	}
	if !ipv4Literal.MatchString(host) {
		return netip.Addr{}, false, false
	}
	addr, ok := parseIPv4Literal(host)
	if !ok {
		return netip.Addr{}, false, true
	}
	return addr, true, false
}

func parseIPv4Literal(host string) (netip.Addr, bool) {
	parts := strings.Split(host, ".")
	if len(parts) < 1 || len(parts) > 4 {
		return netip.Addr{}, false
	}
	nums := make([]uint64, len(parts))
	for i, part := range parts {
		n, ok := parseNum(part)
		if !ok {
			return netip.Addr{}, false
		}
		nums[i] = n
	}
	var ip uint64
	switch len(nums) {
	case 1:
		ip = nums[0]
		if ip > 0xffffffff {
			return netip.Addr{}, false
		}
	case 2:
		if nums[0] > 0xff || nums[1] > 0xffffff {
			return netip.Addr{}, false
		}
		ip = nums[0]<<24 | nums[1]
	case 3:
		if nums[0] > 0xff || nums[1] > 0xff || nums[2] > 0xffff {
			return netip.Addr{}, false
		}
		ip = nums[0]<<24 | nums[1]<<16 | nums[2]
	default:
		if nums[0] > 0xff || nums[1] > 0xff || nums[2] > 0xff || nums[3] > 0xff {
			return netip.Addr{}, false
		}
		ip = nums[0]<<24 | nums[1]<<16 | nums[2]<<8 | nums[3]
	}
	return netip.AddrFrom4([4]byte{byte(ip >> 24), byte(ip >> 16), byte(ip >> 8), byte(ip)}), true
}

func parseNum(part string) (uint64, bool) {
	if part == "" {
		return 0, false
	}
	if len(part) > 2 && part[0] == '0' && (part[1] == 'x' || part[1] == 'X') {
		n, err := strconv.ParseUint(part[2:], 16, 64)
		return n, err == nil
	}
	if len(part) > 1 && part[0] == '0' {
		n, err := strconv.ParseUint(part, 8, 64)
		return n, err == nil
	}
	n, err := strconv.ParseUint(part, 10, 64)
	return n, err == nil
}

// Forbidden — частные, локальные, служебные и групповые адреса.
// IPv4 внутри IPv6 (::ffff:0:0/96) проверяется как обычный IPv4.
func Forbidden(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	addr = addr.Unmap()
	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() || addr.IsInterfaceLocalMulticast() {
		return true
	}
	if !addr.Is4() {
		return false
	}
	return prefixThis.Contains(addr) || prefixCGNAT.Contains(addr) || prefixIETF.Contains(addr) ||
		prefixBench.Contains(addr) || prefixHigh.Contains(addr)
}

// SafeClient не использует прокси и не следует перенаправлениям.
// Перед соединением проверяется IP, который реально выбран для сокета.
func SafeClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   rejectDial,
	}
	return &http.Client{
		CheckRedirect: refuseRedirect,
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         dialer.DialContext,
			ForceAttemptHTTP2:   true,
			TLSHandshakeTimeout: 10 * time.Second,
			// Чат ждёт первый байт до минуты. Список моделей обрывается своим контекстом раньше.
			ResponseHeaderTimeout: 60 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

func refuseRedirect(*http.Request, []*http.Request) error {
	return errors.New("перенаправление запрещено")
}

func rejectDial(network, address string, _ syscall.RawConn) error {
	return checkConnect(address)
}

func checkConnect(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil || Forbidden(ip) {
		return errDenied
	}
	return nil
}
