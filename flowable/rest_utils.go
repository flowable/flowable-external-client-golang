package flowable

import (
	"bytes"
	"io"
	"net/http"
)

// rest_utils.go centralizes HTTP helpers, default headers, and auth settings used by the package.

// Package-level auth and header configuration
var (
	AuthUser       = ""
	AuthPass       = ""
	BearerToken    = ""
	HTTPClient     = &http.Client{}
	DefaultHeaders = map[string]string{
		"Accept":       "application/json",
		"Content-Type": "application/json",
	}
)

// SetAuth allows callers to override the basic auth credentials used for REST requests.
func SetAuth(user, pass string) {
	AuthUser = user
	AuthPass = pass
}

// SetBearerToken allows callers to set a Bearer token for REST requests.
func SetBearerToken(token string) {
	BearerToken = token
}

// SetDefaultHeader sets or overrides a default header key/value for REST requests.
func SetDefaultHeader(key, value string) {
	DefaultHeaders[key] = value
}

// SetHTTPClient allows callers to override the HTTP client used for REST requests.
// Passing nil resets the default client.
func SetHTTPClient(client *http.Client) {
	if client == nil {
		HTTPClient = &http.Client{}
		return
	}
	HTTPClient = client
}

// prepareRequest applies default headers and authentication to an http.Request
func prepareRequest(req *http.Request) {
	for k, v := range DefaultHeaders {
		req.Header.Set(k, v)
	}
	if AuthUser != "" || AuthPass != "" {
		req.SetBasicAuth(AuthUser, AuthPass)
	}
	// set bearer token if available (won't exist on *http.Request)
	if BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+BearerToken)
	}
}

// restGet performs a GET request to the provided full URL and returns status, body bytes, and error.
func restGet(fullURL string) (status int, body []byte, err error) {
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return -1, nil, err
	}

	prepareRequest(req)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return -1, nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, bodyBytes, nil
}

// restPost performs a POST request to the provided full URL with the given JSON payload.
func restPost(fullURL string, payload []byte) (status int, body []byte, err error) {
	req, err := http.NewRequest("POST", fullURL, bytes.NewReader(payload))
	if err != nil {
		return -1, nil, err
	}

	prepareRequest(req)

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return -1, nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, bodyBytes, nil
}
