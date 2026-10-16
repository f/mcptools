package commands

import (
	"fmt"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

func addCustomHTTPHeaders(headers map[string]string, options []string) error {
	for _, option := range options {
		name, value, found := strings.Cut(option, "=")
		if !found || name == "" {
			return fmt.Errorf("--headers requires NAME=VALUE")
		}
		for _, char := range name {
			if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
				(char >= '0' && char <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", char) {
				continue
			}
			return fmt.Errorf("invalid --headers name")
		}
		for _, char := range value {
			if (char < 32 && char != '\t') || char == 127 {
				return fmt.Errorf("invalid --headers value")
			}
		}
		headers[textproto.CanonicalMIMEHeaderKey(name)] = value
	}
	return nil
}

// headerOriginTransport prevents custom headers from being sent to another origin.
// This also covers an SSE endpoint event or an HTTP redirect changing the scheme.
type headerOriginTransport struct {
	origin  *url.URL
	headers map[string]string
}

func (t *headerOriginTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if !strings.EqualFold(request.URL.Scheme, t.origin.Scheme) || !strings.EqualFold(request.URL.Host, t.origin.Host) {
		return nil, fmt.Errorf("refusing to send custom HTTP headers to a different origin")
	}
	forwarded := request.Clone(request.Context())
	if forwarded.Header == nil {
		forwarded.Header = make(http.Header)
	}
	for name, value := range t.headers {
		forwarded.Header.Set(name, value)
	}
	return http.DefaultTransport.RoundTrip(forwarded)
}

func customHeaderClient(rawURL string, headers map[string]string) *http.Client {
	// The transport constructor validates this URL before making a request.
	origin, _ := url.Parse(rawURL)
	return &http.Client{Transport: &headerOriginTransport{origin: origin, headers: headers}}
}
