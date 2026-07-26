package alerts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const dingTalkHost = "oapi.dingtalk.com"

var dingTalkHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

type dingTalkMessage struct {
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

type dingTalkResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func isDingTalkURL(parsedURL *url.URL) bool {
	return parsedURL.Scheme == "dingtalk" ||
		((parsedURL.Scheme == "https" || parsedURL.Scheme == "http") && strings.EqualFold(parsedURL.Hostname(), dingTalkHost))
}

func dingTalkEndpoint(parsedURL *url.URL) (*url.URL, error) {
	if parsedURL.Scheme == "dingtalk" {
		token := parsedURL.Host
		if token == "" {
			token = strings.TrimPrefix(parsedURL.Path, "/")
		}
		if token == "" {
			return nil, errors.New("DingTalk access token is required")
		}
		query := url.Values{"access_token": []string{token}}
		return &url.URL{Scheme: "https", Host: dingTalkHost, Path: "/robot/send", RawQuery: query.Encode()}, nil
	}

	if parsedURL.Scheme != "https" || !strings.EqualFold(parsedURL.Hostname(), dingTalkHost) || parsedURL.Path != "/robot/send" {
		return nil, errors.New("invalid DingTalk webhook URL")
	}
	if parsedURL.Query().Get("access_token") == "" {
		return nil, errors.New("DingTalk access token is required")
	}
	return parsedURL, nil
}

func sendDingTalkAlert(client *http.Client, notificationURL, title, message, link string) error {
	parsedURL, err := url.Parse(notificationURL)
	if err != nil {
		return errors.New("invalid DingTalk webhook URL")
	}
	endpoint, err := dingTalkEndpoint(parsedURL)
	if err != nil {
		return err
	}

	payload := dingTalkMessage{MsgType: "text"}
	payload.Text.Content = title + "\n\n" + message
	if link != "" {
		payload.Text.Content += "\n\n" + link
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode DingTalk message: %w", err)
	}

	request, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return errors.New("create DingTalk request")
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return errors.New("send DingTalk notification failed")
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read DingTalk response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("DingTalk returned HTTP %d", response.StatusCode)
	}
	var result dingTalkResponse
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return errors.New("invalid DingTalk response")
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("DingTalk rejected notification: %s (code %d)", result.ErrMsg, result.ErrCode)
	}
	return nil
}
