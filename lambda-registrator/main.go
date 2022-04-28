package main

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(HandleRequest)
}

func HandleRequest(context.Context, interface{}) (string, error) {
	_, err := SetupEnvironment()

	if err != nil {
		return "", fmt.Errorf("setting up consul environment  %w", err)
	}

	return "", nil
}
