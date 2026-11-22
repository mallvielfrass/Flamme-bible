package server

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// Большие тела проходят через nginx только по своим location.
// Иначе общий client_max_body_size 1m отвечает 413 до API.
func TestNginxLetsLargeBodiesThrough(t *testing.T) {
	routes := map[string]int64{
		"/api/admin/releases":   maxReleaseBody,
		"/api/modules/upload":   maxModuleBody,
		"/api/admin/embed/gguf": defaultGGUFMax,
	}
	for _, file := range []string{"../../web/nginx.conf", "../../deploy/nginx/flammebible.conf.example"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for route, limit := range routes {
			got := nginxBodyLimit(t, string(raw), route)
			if got < limit {
				t.Errorf("%s %s: client_max_body_size %d < %d", file, route, got, limit)
			}
		}
	}
}

func nginxBodyLimit(t *testing.T, conf, route string) int64 {
	t.Helper()
	re := regexp.MustCompile(`location = ` + regexp.QuoteMeta(route) + ` \{[^}]*client_max_body_size (\d+)m;`)
	m := re.FindStringSubmatch(conf)
	if m == nil {
		return 0
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n << 20
}
