package main

import (
	"errors"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	sdklambda "github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/lambda/lambdaiface"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/consul/api"
)

// Environment contains all of Lambda registrator's dependencies.
type Environment struct {
	// Lambda is the Lambda client that will reconcile state with Consul.
	Lambda lambdaiface.LambdaAPI

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
	Partitions []string

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
func SetupEnvironment() (Environment, error) {
	var env Environment

	session := session.Must(session.NewSession())
	ssmClient := ssm.New(session, nil)

	nodeName := os.Getenv(nodeNameEnvironment)
	region := os.Getenv(awsRegionEnvironment)
	isEnterprise := os.Getenv(enterpriseEnvironment) == "true"
	partitionsRaw := os.Getenv(partitionsEnvironment)
	partitions := strings.Split(partitionsRaw, ",")

	logLevel := "info"
	if level := os.Getenv(logLevelEnvironment); level != "" {
		logLevel = level
	}
	logger := hclog.New(
		&hclog.LoggerOptions{
			Level: hclog.LevelFromString(logLevel),
		},
	)

	err := setConsulHTTPToken(ssmClient)
	if err != nil {
		return env, err
	}

	err = setConsulCACert(ssmClient)
	if err != nil {
		return env, err
	}

	cfg := api.DefaultConfig()
	consulClient, err := api.NewClient(cfg)

	if err != nil {
		return env, err
	}

	return Environment{
		Lambda:       sdklambda.New(session),
		NodeName:     nodeName,
		ConsulClient: consulClient,
		Region:       region,
		IsEnterprise: isEnterprise,
		Partitions:   partitions,
		Logger:       logger,
	}, nil
}

func setConsulCACert(ssmClient ssmiface.SSMAPI) error {
	path := os.Getenv(consulCAPathEnvironment)

	if path == "" {
		return nil
	}

	paramValue, err := ssmClient.GetParameter(&ssm.GetParameterInput{
		Name:           &path,
		WithDecryption: aws.Bool(true),
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

func setConsulHTTPToken(ssmClient ssmiface.SSMAPI) error {
	path := os.Getenv(consulHTTPTokenPath)

	if path == "" {
		return nil
	}

	paramValue, err := ssmClient.GetParameter(&ssm.GetParameterInput{
		Name:           &path,
		WithDecryption: aws.Bool(true),
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
