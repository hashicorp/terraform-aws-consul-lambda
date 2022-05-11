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
	env, err := SetupEnvironment(ctx)

	if err != nil {
		// We can't use the logger because of the error.
		fmt.Println("Error setting up the environment: %w", err)
		return "", fmt.Errorf("setting up consul environment:  %w", err)
	}

	events, err := GetEvents(ctx, env, rawEvent)
	if err != nil {
		env.Logger.Warn("Error getting events", "error", err)
		return "", fmt.Errorf("error getting events: %w", err)
	}

	env.Logger.Info("Processing events", "count", len(events))

	var resultErr error

	for _, event := range events {
		err := event.Reconcile(env)

		if err != nil {
			env.Logger.Warn("Error reconciling event", "error", err, "identifier", event.Identifier())
			resultErr = multierror.Append(resultErr, err)
		}
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

		events, err := env.AWSEventToEvents(ctx, e)
		if err != nil {
			return nil, fmt.Errorf("error converting aws.lambda event our event %s", err)
		}

		return events, nil
	}

	return nil, fmt.Errorf("unprocessable event source %q", source)
}
