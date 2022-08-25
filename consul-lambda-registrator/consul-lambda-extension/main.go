package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/hashicorp/go-hclog"
	"github.com/kelseyhightower/envconfig"

	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/client"
	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/extension"
	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/trace"
)

const (
	defaultLogLevel = "info"
	extensionName   = "consul-lambda-extension"
)

func main() {
	trace.Enabled(strings.ToLower(getEnvOrDefault("TRACE_ENABLED", "false")) == "true")

	logger := hclog.New(&hclog.LoggerOptions{
		Level: hclog.LevelFromString(getEnvOrDefault("LOG_LEVEL", defaultLogLevel)),
	})

	err := realMain(logger.Named(extensionName))
	if err != nil {
		logger.Error("fatal error, exiting", "error", err)
		os.Exit(1)
	}
}

func realMain(logger hclog.Logger) error {
	trace.Enter()
	defer trace.Exit()

	trace.SetLogger(trace.HCLog{Logger: logger})

	cfg, err := configure()
	if err != nil {
		return err
	}

	cfg.Logger = logger
	ext := extension.NewExtension(cfg)

	// Handle interrupts and shutdown notification
	ctx, cancel := context.WithCancel(context.Background())
	shutdownChannel := make(chan struct{})
	go func() {
		interruptChannel := make(chan os.Signal, 1)
		signal.Notify(interruptChannel, syscall.SIGTERM, syscall.SIGINT)

		select {
		case s := <-interruptChannel:
			logger.Info("Received signal, exiting", "signal", s)
		case <-shutdownChannel:
			logger.Info("Received shutdown event, exiting")
		}
		// Cancel our context so that all servers and go-routines exit gracefully.
		cancel()
	}()

	err = ext.Serve(ctx)
	if err != nil {
		logger.Error("processing failed with an error", "error", err)
	}

	// Once processEvents returns, signal that it's time to shutdown.
	shutdownChannel <- struct{}{}

	return err
}

func configure() (*extension.Config, error) {
	trace.Enter()
	defer trace.Exit()

	// Load the configuration from the environment.
	cfg := &extension.Config{}
	err := envconfig.Process("consul", cfg)
	if err != nil {
		return cfg, fmt.Errorf("failed to load configuration from environment: %w", err)
	}

	if cfg.ServiceName == "" {
		// If the service name wasn't explicitly configured then default to the Lambda function's name.
		cfg.ServiceName = os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
	}

	sdkConfig, err := config.LoadDefaultConfig(context.Background(), config.WithRetryer(func() aws.Retryer {
		// Adaptive mode should retry on hitting rate limits.
		return retry.AddWithMaxBackoffDelay(retry.NewAdaptiveMode(), 3*time.Second)
	}))
	if err != nil {
		return cfg, fmt.Errorf("failed to create AWS SDK configuration: %w", err)
	}

	ssmClient := client.NewSSM(&sdkConfig)

	lambdaClient := client.NewLambda(&sdkConfig)
	err = lambdaClient.Register(context.Background(), extensionName)
	if err != nil {
		return cfg, fmt.Errorf("failed to register Lambda extension: %w", err)
	}

	cfg.Events = lambdaClient
	cfg.Store = ssmClient
	return cfg, nil
}

func getEnvOrDefault(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
