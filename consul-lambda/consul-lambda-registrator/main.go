// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/hashicorp/go-multierror"
	"github.com/mitchellh/mapstructure"
)

func main() {
	lambda.Start(HandleRequest)
}

func HandleRequest(ctx context.Context, rawEvent map[string]interface{}) (string, error) {
	fmt.Println("[DEBUG] Lambda registrator invoked")
	fmt.Printf("[DEBUG] Raw event: %+v\n", rawEvent)

	env, err := SetupEnvironment(ctx)

	if err != nil {
		// We can't use the logger because of the error.
		fmt.Println("[DEBUG] Error setting up the environment:", err)
		return "", fmt.Errorf("setting up consul environment:  %w", err)
	}

	env.Logger.Info("[DEBUG] Environment setup complete",
		"consul_address", env.ConsulClient.Address(),
		"node_name", env.NodeName,
	)

	events, err := GetEvents(ctx, env, rawEvent)
	if err != nil {
		env.Logger.Warn("[DEBUG] Error getting events", "error", err, "raw_event", rawEvent)
		return "", fmt.Errorf("error getting events: %w", err)
	}

	env.Logger.Info("[DEBUG] Processing events", "count", len(events), "event_types", fmt.Sprintf("%T", events))

	var resultErr error

	for i, event := range events {
		env.Logger.Info("[DEBUG] Processing event", "index", i, "identifier", event.Identifier(), "event_type", fmt.Sprintf("%T", event))
		err := event.Reconcile(env)

		if err != nil {
			env.Logger.Warn("[DEBUG] Error reconciling event", "error", err, "identifier", event.Identifier())
			resultErr = multierror.Append(resultErr, err)
		} else {
			env.Logger.Info("[DEBUG] Successfully reconciled event", "identifier", event.Identifier())
		}
	}

	if resultErr != nil {
		env.Logger.Error("[DEBUG] Handler completed with errors", "error", resultErr)
	} else {
		env.Logger.Info("[DEBUG] Handler completed successfully")
	}

	return "", resultErr
}

type Event interface {
	Reconcile(Environment) error
	Identifier() string
}

func GetEvents(ctx context.Context, env Environment, data map[string]interface{}) ([]Event, error) {
	source, ok := data["source"].(string)
	if !ok {
		return nil, fmt.Errorf("missing event source")
	}

	env.Logger.Info("Received event", "source", source)
	switch source {
	case "aws.events":
		return env.FullSyncData(ctx)
	case "aws.lambda":
		var e AWSEvent
		err := mapstructure.Decode(data, &e)
		if err != nil {
			return nil, fmt.Errorf("error decoding aws.lambda event %s", err)
		}

		if e.Detail.ErrorCode != "" {
			env.Logger.Info("Unprocessable event",
				"errorCode", e.Detail.ErrorCode,
				"eventID", e.Detail.EventID,
				"eventName", e.Detail.EventName)
			return nil, nil
		}

		events, err := env.AWSEventToEvents(ctx, e)
		if err != nil {
			return nil, fmt.Errorf("error converting aws.lambda event our event %s", err)
		}

		return events, nil
	}

	return nil, fmt.Errorf("unprocessable event source %q", source)
}
