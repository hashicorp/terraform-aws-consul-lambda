// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/kelseyhightower/envconfig"

	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/client"
)

// Config holds the configuration from the environment.
type Config struct {
	// NodeName is the Consul node name that will have all Lambda services registered to it.
	NodeName string `envconfig:"NODE_NAME" required:"true"`

	// Datacenter is the Consul datacenter that the Lambda registrator manages.
	// If not set, Lambda registrator will manage Lambda services for all datacenters in this region.
	Datacenter string `envconfig:"DATACENTER"`

	// IsEnterprise is used to determine if Consul is OSS or Enterprise.
	IsEnterprise bool `envconfig:"ENTERPRISE" default:"false"`

	// RawPartitions is the raw, comma-separated string list of partitions.
	RawPartitions []string `envconfig:"PARTITIONS"`

	// Partitions specifies the Admin Partitions that Lambda registrator manages.
	Partitions map[string]struct{} `ignored:"true"`

	// LogLevel is the configured logging level.
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`

	// ConsulCACertPath is the path to the Consul CA cert in Parameter Store.
	ConsulCACertPath string `envconfig:"CONSUL_CACERT_PATH"`

	// ConsulHTTPToken is the path to the Consul HTTP token in Parameter Store.
	ConsulHTTPTokenPath string `envconfig:"CONSUL_HTTP_TOKEN_PATH"`

	// ExtensionDataPrefix is the path in Parameter Store where extension data will be written.
	ExtensionDataPrefix string `envconfig:"CONSUL_EXTENSION_DATA_PREFIX"`

	// PageSize is the maximum number of Lambda functions per page when querying the Lambda API.
	PageSize int `envconfig:"PAGE_SIZE" default:"50"`
}

// initPartitions converts the raw slice of partitions into a map.
func (c *Config) initPartitions() {
	c.Partitions = make(map[string]struct{}, len(c.RawPartitions))
	for _, p := range c.RawPartitions {
		c.Partitions[p] = struct{}{}
	}
}

// ParamStore is an interface for reading and writing key/value pairs to a data store.
type ParamStore interface {
	Delete(ctx context.Context, k string) error
	Get(ctx context.Context, k string) (string, error)
	Set(ctx context.Context, k, v string) error
}

// LambdaAPIClient is an interface for retrieving information about Lambda functions.
type LambdaAPIClient interface {
	GetFunction(context.Context, string) (LambdaFunction, error)
	ListFunctions(context.Context) (map[string]LambdaFunction, error)
}

// Environment contains all of Lambda registrator's dependencies.
type Environment struct {
	Config

	// ConsulClient is the Consul client that will be kept in sync with managed Lambda state.
	ConsulClient *api.Client

	// Lambda is the Lambda client used to interact with the Lambda API.
	Lambda LambdaAPIClient

	// Logger is used to log messages.
	Logger hclog.Logger

	// Store is data store client used to read and write configuration data.
	Store ParamStore
}

const (
	caCertPath string = "/tmp/consul-ca-cert.pem"
)

// SetupEnvironment constructs the processing Environment based on environment variables
// and Parameter Store.
func SetupEnvironment(ctx context.Context) (Environment, error) {
	var env Environment

	err := envconfig.Process("", &env)
	if err != nil {
		return env, err
	}
	env.initPartitions()

	env.Logger = hclog.New(
		&hclog.LoggerOptions{
			Level: hclog.LevelFromString(env.LogLevel),
		},
	)

	sdkConfig, err := config.LoadDefaultConfig(ctx, config.WithRetryer(func() aws.Retryer {
		// Adaptive mode should retry on hitting rate limits.
		return retry.AddWithMaxBackoffDelay(retry.NewAdaptiveMode(), 3*time.Second)
	}))
	if err != nil {
		return env, err
	}

	advancedTier, err := strconv.ParseBool(os.Getenv("CONSUL_ADVANCED_PARAMS"))
	if err != nil {
		env.Logger.Debug("Unable to parse (true, false) setting to standard tier parameter")
	}

	env.Store = client.NewSSM(&sdkConfig, advancedTier)
	env.Lambda = NewLambdaClient(&sdkConfig, env.PageSize)

	err = setConsulHTTPToken(ctx, env.Store, env.ConsulHTTPTokenPath)
	if err != nil {
		return env, err
	}

	err = setConsulCACert(ctx, env.Store, env.ConsulCACertPath)
	if err != nil {
		return env, err
	}

	env.ConsulClient, err = api.NewClient(api.DefaultConfig())
	if err != nil {
		return env, err
	}

	return env, nil
}

// IsManagingTLS indicates whether the Environment is configured to retrieve mTLS data from Consul and
// write it to the parameter store.
func (env Environment) IsManagingTLS() bool {
	return len(env.ExtensionDataPrefix) > 0
}

func setConsulCACert(ctx context.Context, store ParamStore, path string) error {
	if path == "" {
		return nil
	}

	cert, err := store.Get(ctx, path)
	if err != nil {
		return err
	}

	err = os.WriteFile(caCertPath, []byte(cert), 0666)
	if err != nil {
		return err
	}

	return os.Setenv("CONSUL_CACERT", caCertPath)
}

func setConsulHTTPToken(ctx context.Context, store ParamStore, path string) error {
	if path == "" {
		return nil
	}

	token, err := store.Get(ctx, path)
	if err != nil {
		return err
	}

	return os.Setenv("CONSUL_HTTP_TOKEN", token)
}
