package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
)

func TestForbidden(t *testing.T) {
	deny := []string{
		"0.0.0.0", "0.1.2.3", "10.0.0.1", "100.64.0.1", "100.127.255.255",
		"127.0.0.1", "169.254.1.1", "172.16.0.1", "172.31.255.255",
		"192.0.0.1", "192.0.0.8", "192.168.1.1", "198.18.0.1", "198.19.255.255",
		"224.0.0.1", "240.0.0.1", "255.255.255.255",
		"::", "::1", "fc00::1", "fd00::1", "fe80::1", "ff00::1", "ff02::1",
		"::ffff:127.0.0.1", "::ffff:10.1.2.3", "::ffff:169.254.1.1",
	}
	allow := []string{
		"203.0.113.10", "100.63.255.255", "100.128.0.1", "172.32.0.1",
		"192.0.1.1", "198.20.0.1", "223.255.255.255", "192.0.2.1",
		"2001:db8::1", "::ffff:203.0.113.10",
	}
	for _, raw := range deny {
		addr := netip.MustParseAddr(raw)
		if !Forbidden(addr) {
			t.Errorf("пропущен %s", raw)
		}
	}
	for _, raw := range allow {
		addr := netip.MustParseAddr(raw)
		if Forbidden(addr) {
			t.Errorf("запрещён %s", raw)
		}
	}
}

func TestValidateBaseURL(t *testing.T) {
	var seen []string
	resolve := func(ctx context.Context, host string) ([]netip.Addr, error) {
		seen = append(seen, host)
		switch strings.TrimSuffix(host, ".") {
		case "example.com":
			return []netip.Addr{netip.MustParseAddr("203.0.113.10")}, nil
		case "rebind.example.com":
			return []netip.Addr{
				netip.MustParseAddr("203.0.113.10"),
				netip.MustParseAddr("10.1.2.3"),
			}, nil
		case "down.example.com":
			return nil, errors.New("нет адреса")
		case "empty.example.com":
			return nil, nil
		default:
			t.Errorf("не ждали DNS для %s", host)
			return nil, errors.New("лишний запрос")
		}
	}
	ok := []struct{ raw, want string }{
		{"https://example.com/v1", "https://example.com/v1"},
		{"  https://EXAMPLE.com:8443/v1/ ", "https://example.com:8443/v1"},
		{"https://example.com./v1/", "https://example.com/v1"},
		{"https://203.0.113.10/v1", "https://203.0.113.10/v1"},
		{"https://[2001:db8::1]/v1", "https://[2001:db8::1]/v1"},
		{"https://[::ffff:203.0.113.10]/v1", "https://203.0.113.10/v1"},
	}
	bad := []string{
		"http://example.com/v1",
		"HTTP://example.com/v1",
		"https://user:pass@example.com/v1",
		"https://user@example.com/v1",
		"https://example.com/v1?x=1",
		"https://example.com/v1#x",
		"ftp://example.com",
		"https://localhost/v1",
		"https://LOCALHOST/v1",
		"https://127.0.0.1/v1",
		"https://127.0.0.1:8443/v1",
		"https://127.1/",
		"https://0x7f000001/",
		"https://2130706433/",
		"https://0177.0.0.1/",
		"https://0x7f.0.0.1/",
		"https://[::ffff:127.0.0.1]/",
		"https://[::1]/",
		"https://10.1.2.3/",
		"https://192.168.0.1/",
		"https://172.16.5.1/",
		"https://169.254.169.254/",
		"https://0.0.0.0/",
		"https://0.1.2.3/",
		"https://100.64.1.1/",
		"https://192.0.0.8/",
		"https://198.18.1.1/",
		"https://224.0.0.1/",
		"https://255.255.255.255/",
		"https://[fc00::1]/",
		"https://[fe80::1]/",
		"https://[ff02::1]/",
		"https://rebind.example.com/v1",
		"https://down.example.com/v1",
		"https://empty.example.com/v1",
		"https://example.com/v1 path",
		"",
		"https://",
	}
	for _, item := range ok {
		seen = nil
		got, err := ValidateBaseURLWith(context.Background(), item.raw, resolve)
		if err != nil {
			t.Errorf("%s: %v", item.raw, err)
			continue
		}
		if got != item.want {
			t.Errorf("%s -> %s, ждали %s", item.raw, got, item.want)
		}
	}
	for _, raw := range bad {
		seen = nil
		got, err := ValidateBaseURLWith(context.Background(), raw, resolve)
		if err == nil {
			t.Errorf("принят %s -> %s", raw, got)
		}
	}
	seen = nil
	if _, err := ValidateBaseURLWith(context.Background(), "https://127.1/v1", resolve); err == nil {
		t.Fatal("127.1 принят")
	}
	if len(seen) != 0 {
		t.Fatalf("127.1 ушёл в DNS: %v", seen)
	}
	seen = nil
	if _, err := ValidateBaseURLWith(context.Background(), "https://localhost/v1", resolve); err == nil {
		t.Fatal("localhost принят")
	}
	if len(seen) != 0 {
		t.Fatalf("localhost ушёл в DNS: %v", seen)
	}
	seen = nil
	if _, err := ValidateBaseURLWith(context.Background(), "https://0x7f000001/", resolve); err == nil {
		t.Fatal("hex принят")
	}
	if len(seen) != 0 {
		t.Fatalf("hex ушёл в DNS: %v", seen)
	}
	seen = nil
	if _, err := ValidateBaseURLWith(context.Background(), "https://[::ffff:127.0.0.1]/", resolve); err == nil {
		t.Fatal("mapped принят")
	}
	if len(seen) != 0 {
		t.Fatalf("mapped ушёл в DNS: %v", seen)
	}
}

func TestSafeClient(t *testing.T) {
	client := SafeClient()
	if client.CheckRedirect == nil {
		t.Fatal("нет запрета перенаправлений")
	}
	if err := client.CheckRedirect(nil, nil); err == nil {
		t.Fatal("перенаправление разрешено")
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok || tr.Proxy != nil {
		t.Fatal("прокси не nil")
	}
	if err := checkConnect("10.1.2.3:443"); err == nil {
		t.Fatal("10.x пропущен на соединении")
	}
	if err := checkConnect("[::ffff:127.0.0.1]:443"); err == nil {
		t.Fatal("mapped loopback пропущен на соединении")
	}
	if err := checkConnect("203.0.113.10:443"); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, err = client.Get("https://" + ln.Addr().String() + "/")
	if !errors.Is(err, errDenied) {
		t.Fatalf("соединение с loopback: %v", err)
	}
}
