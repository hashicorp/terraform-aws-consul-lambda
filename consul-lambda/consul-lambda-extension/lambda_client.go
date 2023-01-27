package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const (
	fmtExtensionURL     = "http://%s/2020-01-01/extension"
	headerExtensionName = "Lambda-Extension-Name"
	headerExtensionID   = "Lambda-Extension-Identifier"
)

// Lambda is a client for interfacing with AWS Lambda APIs.
type Lambda struct {
	baseURL     string
	httpClient  *http.Client
	extensionID string
}

// RegisterResponse is the body of the response for /register
type RegisterResponse struct {
	FunctionName    string            `json:"functionName"`
	FunctionVersion string            `json:"functionVersion"`
	Handler         string            `json:"handler"`
	Configuration   map[string]string `json:"configuration"`
}

// NextEventResponse is the response for /event/next
type NextEventResponse struct {
	EventType          EventType `json:"eventType"`
	DeadlineMs         int64     `json:"deadlineMs"`
	RequestID          string    `json:"requestId"`
	InvokedFunctionArn string    `json:"invokedFunctionArn"`
	Tracing            Tracing   `json:"tracing"`
}

// Tracing is part of the response for /event/next
type Tracing struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// EventType represents the type of events received from /event/next
type EventType string

const (
	Shutdown EventType = "SHUTDOWN"
)

// NewLambda returns a Lambda client for interacting with the Lambda runtime extension API.
func NewLambda() *Lambda {
	baseURL := fmt.Sprintf(fmtExtensionURL, os.Getenv("AWS_LAMBDA_RUNTIME_API"))
	l := &Lambda{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
	return l
}

// ProcessEvents polls the Lambda Extension API for events. Currently all this
// does is signal readiness to the Lambda platform after each event, which is
// required in the Extension API.
// The first call to NextEvent signals completion of the extension
// init phase.
func (c *Lambda) ProcessEvents(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			res, err := c.next(ctx)
			if err != nil {
				return fmt.Errorf("failed to receive next event: %w", err)
			}
			// Exit if we receive a SHUTDOWN event
			if res.EventType == Shutdown {
				return nil
			}
		}
	}
}

// Register the named extension with the Lambda Extensions API
// The interface value i is the name of the extension to register as a string.
// If i is not a string a non-nil error is returned.
func (c *Lambda) Register(ctx context.Context, i interface{}) error {
	const action = "/register"
	url := c.baseURL + action

	name, ok := i.(string)
	if !ok {
		return fmt.Errorf("invalid input type, expected string")
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"events": []EventType{Shutdown},
	})
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	httpReq.Header.Set(headerExtensionName, name)
	httpRes, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	if httpRes.StatusCode != 200 {
		return fmt.Errorf("extension registration request failed with status %s", httpRes.Status)
	}
	c.extensionID = httpRes.Header.Get(headerExtensionID)
	return nil
}

// next blocks while long polling for the next lambda invoke or shutdown
func (c *Lambda) next(ctx context.Context) (*NextEventResponse, error) {
	const action = "/event/next"
	url := c.baseURL + action

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set(headerExtensionID, c.extensionID)
	httpRes, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if httpRes.StatusCode != 200 {
		return nil, fmt.Errorf("request failed with status %s", httpRes.Status)
	}
	defer httpRes.Body.Close()
	body, err := io.ReadAll(httpRes.Body)
	if err != nil {
		return nil, err
	}
	res := NextEventResponse{}
	err = json.Unmarshal(body, &res)
	if err != nil {
		return nil, err
	}
	return &res, nil
}
