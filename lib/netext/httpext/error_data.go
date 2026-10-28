package httpext

import (
	"maps"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Sample metadata keys holding the request and response of a failed request. They are
// written only on the http_req_failed sample, and only when the HTTPErrorData option is
// enabled.
const (
	MetadataErrorReqHeaders = "error_req_headers"
	MetadataErrorReqBody    = "error_req_body"
	MetadataErrorResHeaders = "error_res_headers"
	MetadataErrorResBody    = "error_res_body"
)

// maxErrorBodyLen caps each body in the error metadata, which every output receives.
const maxErrorBodyLen = 16 << 10

// redactedHeaders carry credentials and are never written to the error metadata.
var redactedHeaders = []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie"}

// errorMetadata returns a copy of meta extended with the request and response of a
// failed request. It is a copy so the data lands only on the http_req_failed sample,
// not on every sample of the trail that shares meta. res is nil when no response was
// received.
func errorMetadata(
	meta map[string]string, req *http.Request, res *http.Response, reqURL, reqBody, resBody string,
) map[string]string {
	m := maps.Clone(meta)
	if m == nil {
		m = make(map[string]string, 4)
	}
	// k6 leaves req.Proto empty and lets the transport pick the protocol, so the
	// response is the only record of what was actually spoken.
	proto := "HTTP/1.1"
	if res != nil && res.Proto != "" {
		proto = res.Proto
	}
	m[MetadataErrorReqHeaders] = req.Method + " " + reqURL + " " + proto + formatHeader(req.Header)
	if res != nil {
		m[MetadataErrorResHeaders] = res.Proto + " " + res.Status + formatHeader(res.Header)
	}
	if reqBody != "" {
		m[MetadataErrorReqBody] = errorBody(reqBody)
	}
	if resBody != "" {
		m[MetadataErrorResBody] = errorBody(resBody)
	}
	return m
}

// formatHeader renders h as newline-prefixed "Name: value" lines, sorted by name.
func formatHeader(h http.Header) string {
	h = h.Clone()
	for _, name := range redactedHeaders {
		if _, ok := h[name]; ok {
			h[name] = []string{"[redacted]"}
		}
	}
	var b strings.Builder
	_ = h.Write(&b) // a strings.Builder never fails
	if b.Len() == 0 {
		return ""
	}
	return "\n" + strings.ReplaceAll(strings.TrimSuffix(b.String(), "\r\n"), "\r\n", "\n")
}

// errorBody makes body safe for metadata: binary content is replaced and long
// content is cut at a rune boundary.
func errorBody(body string) string {
	if !utf8.ValidString(body) {
		return "<binary>"
	}
	if len(body) <= maxErrorBodyLen {
		return body
	}
	i := maxErrorBodyLen
	for !utf8.RuneStart(body[i]) {
		i--
	}
	return body[:i] + "...[truncated]"
}

// responseBodyString returns the body MakeRequest read, which is a string or []byte
// depending on the response type, or nil when bodies are discarded.
func responseBodyString(body any) string {
	switch b := body.(type) {
	case string:
		return b
	case []byte:
		return string(b)
	}
	return ""
}
