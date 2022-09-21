package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/hashicorp/go-multierror"
)

type LambdaFunction struct {
	ARN  string
	Name string
	Tags map[string]string
}

// Lambda is a client for interfacing with the AWS Lambda API.
type Lambda struct {
	lambdaClient *lambda.Client
	pageSize     int
}

// NewLambdaClient returns a Lambda client
func NewLambdaClient(cfg *aws.Config, pageSize int) *Lambda {
	return &Lambda{
		lambdaClient: lambda.NewFromConfig(*cfg),
		pageSize:     pageSize,
	}
}

func (c *Lambda) GetFunction(ctx context.Context, arn string) (LambdaFunction, error) {
	fn, err := c.lambdaClient.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: &arn,
	})
	if err != nil {
		return LambdaFunction{}, err
	}

	return LambdaFunction{
		ARN:  *fn.Configuration.FunctionArn,
		Name: *fn.Configuration.FunctionName,
		Tags: fn.Tags,
	}, nil
}

// ListFunctions returns a map of LambdaFunction indexed by ARN.
func (c *Lambda) ListFunctions(ctx context.Context) (map[string]LambdaFunction, error) {
	var resultErr error
	params := &lambda.ListFunctionsInput{MaxItems: aws.Int32(int32(c.pageSize))}
	paginator := lambda.NewListFunctionsPaginator(c.lambdaClient, params)
	lambdas := make(map[string]LambdaFunction)

	for paginator.HasMorePages() {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			resultErr = multierror.Append(resultErr, err)
			return nil, resultErr
		}

		// TODO: fetch Lambda functions concurrently
		for _, l := range output.Functions {
			fn, err := c.GetFunction(ctx, *l.FunctionArn)
			if err != nil {
				resultErr = multierror.Append(resultErr, err)
				continue
			}

			lambdas[fn.ARN] = fn
		}
	}

	return lambdas, nil
}
