package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul/api"
)

// Environment contains all of Lambda registrator's dependencies.
type Environment struct {
	// Lambda is the Lambda client that will reconcile state with Consul.
	Lambda LambdaAPIClient

	// NodeName is the Consul node name that will have all Lambda services
	// registered to it.
	NodeName string

	// ConsulClient is the consul client that will be kept in sync with Lambda's
	// APIs.
	ConsulClient *api.Client

	// Region is the AWS region Lambda registrator is running in.
	Region string

	// IsEnterprise is used to determine if Consul is OSS or Enterprise.
	IsEnterprise bool

	// Partitions specifies the Admin Partitions that Lambda registrator manages.
	Partitions map[string]struct{}

	Logger hclog.Logger
}

const (
	nodeNameEnvironment     string = "NODE_NAME"
	awsRegionEnvironment    string = "AWS_REGION"
	enterpriseEnvironment   string = "ENTERPRISE"
	partitionsEnvironment   string = "PARTITIONS"
	logLevelEnvironment     string = "LOG_LEVEL"
	consulCAPathEnvironment string = "CONSUL_CACERT_PATH"
	consulHTTPTokenPath     string = "CONSUL_HTTP_TOKEN_PATH"
)

const (
	caCertPath string = "/tmp/consul-ca-cert.pem"
)

// SetupEnvironment constructs Environment based on environment variables
// and Parameter Store.
func SetupEnvironment(ctx context.Context) (Environment, error) {
	var env Environment

	sdkConfig, err := config.LoadDefaultConfig(ctx, config.WithRetryer(func() aws.Retryer {
		// Adaptive mode should retry on hitting rate limits.
		return retry.AddWithMaxBackoffDelay(retry.NewAdaptiveMode(), 3*time.Second)
	}))
	if err != nil {
		return env, err
	}

	ssmClient := ssm.NewFromConfig(sdkConfig)

	nodeName := os.Getenv(nodeNameEnvironment)
	region := os.Getenv(awsRegionEnvironment)
	isEnterprise := os.Getenv(enterpriseEnvironment) == "true"
	partitionsRaw := os.Getenv(partitionsEnvironment)

	partitions := make(map[string]struct{})
	for _, p := range strings.Split(partitionsRaw, ",") {
		partitions[p] = struct{}{}
	}

	logLevel := "info"
	if level := os.Getenv(logLevelEnvironment); level != "" {
		logLevel = level
	}
	logger := hclog.New(
		&hclog.LoggerOptions{
			Level: hclog.LevelFromString(logLevel),
		},
	)

	err = setConsulHTTPToken(ctx, ssmClient)
	if err != nil {
		return env, err
	}

	err = setConsulCACert(ctx, ssmClient)
	if err != nil {
		return env, err
	}

	consulConfig := api.DefaultConfig()
	consulClient, err := api.NewClient(consulConfig)

	if err != nil {
		return env, err
	}

	return Environment{
		Lambda:       lambda.NewFromConfig(sdkConfig),
		NodeName:     nodeName,
		ConsulClient: consulClient,
		Region:       region,
		IsEnterprise: isEnterprise,
		Partitions:   partitions,
		Logger:       logger,
	}, nil
}

type GetParameterAPIClient interface {
	GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

type LambdaAPIClient interface {
	lambda.GetFunctionAPIClient
	lambda.ListFunctionsAPIClient
}

func setConsulCACert(ctx context.Context, ssmClient GetParameterAPIClient) error {
	path := os.Getenv(consulCAPathEnvironment)

	if path == "" {
		return nil
	}

	paramValue, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &path,
		WithDecryption: true,
	})

	if err != nil {
		return err
	}

	value := paramValue.Parameter.Value
	if value == nil {
		return errors.New("no parameter store value for conusl cacert arn")
	}

	err = os.WriteFile(caCertPath, []byte(*value), 0666)
	if err != nil {
		return err
	}

	os.Setenv("CONSUL_CACERT", caCertPath)

	return nil
}

func setConsulHTTPToken(ctx context.Context, ssmClient GetParameterAPIClient) error {
	path := os.Getenv(consulHTTPTokenPath)

	if path == "" {
		return nil
	}

	paramValue, err := ssmClient.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &path,
		WithDecryption: true,
	})

	if err != nil {
		return err
	}

	value := paramValue.Parameter.Value
	if value == nil {
		return errors.New("no parameter store value for for conusl http token")
	}

	os.Setenv("CONSUL_HTTP_TOKEN", *value)

	return nil
}
