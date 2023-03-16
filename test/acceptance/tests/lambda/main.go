// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(HandleRequest)
}

// With payload_passthrough = true
//	{
//	  "lambda-to-mesh": true
//	}
//
// With payload_passthrough = false
//	{
//	  "body": {
//	    "lambda-to-mesh": true
//	  }
//	}
func HandleRequest(i interface{}) (map[string]interface{}, error) {

	request, ok := i.(map[string]interface{})
	if !ok {
		// If the request body is not an object then assume the mesh-to-lambda use case
		// to handle a primitive request body or no body at all.
		return meshToLambda(i)
	}

	// If payload_passthrough is true, then the request body is the root object.
	body := request

	// If payload_passthrough = false then there must be a "body" field.
	if b, ok := request["body"]; ok {
		if bb, ok := b.(map[string]interface{}); ok {
			body = bb
		} else {
			// Not an object so call the mesh-to-lambda use case.
			return meshToLambda(b)
		}
	}

	// Check to see if the "lambda-to-mesh" field was provided.
	// Note that the value of the field is not checked.
	if _, ok = body["lambda-to-mesh"]; ok {
		return lambdaToMesh()
	}

	return meshToLambda(body)
}

// meshToLambda handles the case of a Consul mesh service calling this Lambda function.
// It echoes the request body in its response body.
func meshToLambda(i interface{}) (map[string]interface{}, error) {
	// Copy the request body to the response body
	response := make(map[string]interface{})
	response["statusCode"] = http.StatusOK

	if i == nil {
		return response, nil
	}

	response["body"] = i
	return response, nil
}

type UpstreamResponse struct {
	Name string                 `json:"name"`
	Body map[string]interface{} `json:"body"`
	Code int                    `json:"code"`
}

// UpstreamResponseList implements sort.Interface to ensure that responses can be
// returned in a deterministic order. The requests are made to all the upstreams in
// parallel and the responses may come back in a mixed order.
type UpstreamResponseList []UpstreamResponse

func (l UpstreamResponseList) Len() int           { return len(l) }
func (l UpstreamResponseList) Less(i, j int) bool { return l[i].Name < l[j].Name }
func (l UpstreamResponseList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

// lambdaToMesh handles the case of a Lambda function calling into to services within
// the Consul service mesh. It returns the responses from the upstream services in
// its response body.
func lambdaToMesh() (map[string]interface{}, error) {

	upstreams := strings.Split(os.Getenv("UPSTREAMS"), ",")

	respChan := make(chan UpstreamResponse, len(upstreams))
	errChan := make(chan error)

	// Call upstreams concurrently
	for _, upstream := range upstreams {
		go func(upstream string, rc chan UpstreamResponse, ec chan error) {
			r, err := getUpstreamResponse(upstream)
			if err != nil {
				ec <- fmt.Errorf("failed to get upstream response: %w", err)
				return
			}
			rc <- r
		}(upstream, respChan, errChan)
	}

	response := make(map[string]interface{})
	response["statusCode"] = http.StatusInternalServerError

	// Collect responses.
	responses := make(UpstreamResponseList, 0, len(upstreams))
	for i := 0; i < len(upstreams); i++ {
		select {
		case r := <-respChan:
			responses = append(responses, r)
		case e := <-errChan:
			fmt.Println("processing failed:", e)
			return response, fmt.Errorf("failed to process request: %w", e)
		}
	}

	// Sort the responses so that they are always returned in a deterministic order.
	sort.Sort(responses)

	response["statusCode"] = http.StatusOK
	response["body"] = responses

	return response, nil
}

func getUpstreamResponse(u string) (UpstreamResponse, error) {
	ur := UpstreamResponse{Name: u, Code: http.StatusInternalServerError}

	r, err := http.Get(u)
	if err != nil {
		ur.Body = map[string]interface{}{"error": fmt.Sprintf("failed to get %s", u)}
		return ur, err
	}
	defer r.Body.Close()

	b, err := io.ReadAll(r.Body)
	if err != nil {
		ur.Body = map[string]interface{}{"error": "failed to read response body"}
		return ur, err
	}

	if err = json.Unmarshal(b, &ur.Body); err != nil {
		ur.Body = map[string]interface{}{"error": fmt.Sprintf("failed to unmarshal response: %s", err.Error())}
		return ur, err
	}

	ur.Code = r.StatusCode
	return ur, err
}
