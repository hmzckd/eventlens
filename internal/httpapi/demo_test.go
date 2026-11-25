package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDemoPageCarriesTheWriteKeyAsAJavaScriptString(t *testing.T) {
	h := NewHandler(Options{Logger: discardLogger(), DB: fakePinger{}, WriteKey: `key"</script>`, Sink: &recordingSink{}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/demo", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q", ct)
	}
	body := rec.Body.String()
	if strings.Contains(body, `key"</script>`) {
		t.Fatal("the write key was written into the page without escaping")
	}
	if !strings.Contains(body, `const writeKey = "key\"\u003c/script\u003e";`) {
		t.Fatalf("write key not embedded as a JS string, page has: %s", body[strings.Index(body, "const writeKey"):][:60])
	}
	if !strings.Contains(body, `"p-001"`) {
		t.Fatal("product list missing")
	}
}
