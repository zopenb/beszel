package alerts

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestDingTalkEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantToken string
		wantError string
	}{
		{name: "service URL", input: "dingtalk://token-value", wantToken: "token-value"},
		{name: "official webhook", input: "https://oapi.dingtalk.com/robot/send?access_token=token-value", wantToken: "token-value"},
		{name: "missing token", input: "dingtalk://", wantError: "access token is required"},
		{name: "insecure official webhook", input: "http://oapi.dingtalk.com/robot/send?access_token=token-value", wantError: "invalid DingTalk webhook URL"},
		{name: "wrong path", input: "https://oapi.dingtalk.com/other?access_token=token-value", wantError: "invalid DingTalk webhook URL"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsedURL, err := url.Parse(test.input)
			require.NoError(t, err)
			endpoint, err := dingTalkEndpoint(parsedURL)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "https", endpoint.Scheme)
			assert.Equal(t, dingTalkHost, endpoint.Host)
			assert.Equal(t, "/robot/send", endpoint.Path)
			assert.Equal(t, test.wantToken, endpoint.Query().Get("access_token"))
		})
	}
}

func TestSendDingTalkAlert(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, dingTalkHost, request.URL.Host)
		assert.Equal(t, "/robot/send", request.URL.Path)
		assert.Equal(t, "test-token", request.URL.Query().Get("access_token"))
		assert.Equal(t, "application/json", request.Header.Get("Content-Type"))

		var payload dingTalkMessage
		require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
		assert.Equal(t, "text", payload.MsgType)
		assert.Equal(t, "CPU alert\n\nUsage exceeded 90%\n\nhttps://beszel.example/system/1", payload.Text.Content)

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"errcode":0,"errmsg":"ok"}`)),
		}, nil
	})}

	err := sendDingTalkAlert(client, "dingtalk://test-token", "CPU alert", "Usage exceeded 90%", "https://beszel.example/system/1")
	require.NoError(t, err)
}

func TestSendDingTalkAlertRejectsApplicationError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"errcode":310000,"errmsg":"invalid token"}`)),
		}, nil
	})}

	err := sendDingTalkAlert(client, "dingtalk://test-token", "Alert", "Message", "")
	require.ErrorContains(t, err, "invalid token (code 310000)")
}

func TestSendDingTalkAlertRejectsHTTPError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("bad gateway")),
		}, nil
	})}

	err := sendDingTalkAlert(client, "dingtalk://test-token", "Alert", "Message", "")
	require.ErrorContains(t, err, "HTTP 502")
}

func TestSendDingTalkAlertRedactsTokenFromNetworkError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network error")
	})}

	err := sendDingTalkAlert(client, "dingtalk://secret-token", "Alert", "Message", "")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-token")
}
