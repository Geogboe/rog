package selfupdate

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"testing"
)

// testCtx returns a context for use in tests.
func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}

// testGOOS returns runtime.GOOS for use in constructing expected asset names in tests.
func testGOOS() string { return runtime.GOOS }

// testGOARCH returns runtime.GOARCH for use in constructing expected asset names in tests.
func testGOARCH() string { return runtime.GOARCH }

// prefixedClient rewrites GitHub API URLs to point to a test server,
// while passing through all other URLs unchanged.
type prefixedClient struct {
	base      *http.Client
	urlPrefix string // test server base URL, e.g. "http://127.0.0.1:PORT"
	apiBase   string // real API base to rewrite, e.g. "https://api.github.com"
}

func (c *prefixedClient) Do(req *http.Request) (*http.Response, error) {
	if strings.HasPrefix(req.URL.String(), c.apiBase) {
		rewritten := c.urlPrefix + req.URL.Path
		if req.URL.RawQuery != "" {
			rewritten += "?" + req.URL.RawQuery
		}
		newReq := req.Clone(req.Context())
		newReq.URL, _ = req.URL.Parse(rewritten)
		req = newReq
	}
	return c.base.Do(req)
}
