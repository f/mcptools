package commands

import (
	"fmt"
	"net/http"
	"os"
)

var verbose = &http.Client{
	Transport: verbosed{http.DefaultTransport},
}

type verbosed struct {
	impl http.RoundTripper
}

func verboseHeader(dir byte, header map[string][]string) {
	for k, a := range header {
		for _, v := range a {
			fmt.Fprintf(os.Stderr, "%c %s: %s\n", dir, k, v)
		}
	}
}

func (v verbosed) RoundTrip(req *http.Request) (*http.Response, error) {
	verboseHeader('>', req.Header)
	fmt.Fprintf(os.Stderr, "> \n")
	resp, err := v.impl.RoundTrip(req)
	if resp != nil {
		verboseHeader('<', resp.Header)
		fmt.Fprintf(os.Stderr, "< \n")
	}
	fmt.Fprintf(os.Stderr, "\n")
	return resp, err
}
