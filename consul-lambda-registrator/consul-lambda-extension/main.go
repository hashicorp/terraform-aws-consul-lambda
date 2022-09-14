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
	logger := hclog.New(&hclog.LoggerOptions{
		Level: hclog.LevelFromString(getEnvOrDefault("LOG_LEVEL", defaultLogLevel)),
	})

	trace.SetLogger(trace.NewHCLog(logger, hclog.Info))
	trace.Enabled(strings.ToLower(getEnvOrDefault("TRACE_ENABLED", "false")) == "true")

	err := realMain(logger.Named(extensionName))
	if err != nil {
		logger.Error("fatal error, exiting", "error", err)
		os.Exit(1)
	}
}

func realMain(logger hclog.Logger) error {
	trace.Enter()
	defer trace.Exit()

	cfg, err := configure()
	if err != nil {
		return err
	}

	cfg.Logger = logger
	ext := extension.NewExtension(cfg)

	// Handle interrupts and shutdown notification
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		interruptChannel := make(chan os.Signal, 1)
		signal.Notify(interruptChannel, syscall.SIGTERM, syscall.SIGINT)
		defer signal.Stop(interruptChannel)

		select {
		case s := <-interruptChannel:
			logger.Info("Received signal, exiting", "signal", s)
		case <-ctx.Done():
			logger.Info("Received shutdown event, exiting")
		}
		// Cancel our context so that all servers and go-routines exit gracefully.
		cancel()
	}()

	err = ext.Start(ctx)
	if err != nil {
		logger.Error("processing failed with an error", "error", err)
	}

	// Signal that it's time to shutdown when the extension returns.
	cancel()

	return err
}

func configure() (*extension.Config, error) {
	trace.Enter()
	defer trace.Exit()

	// Load the configuration from the environment.
	cfg := &extension.Config{}
	err := envconfig.Process("", cfg)
	if err != nil {
		return cfg, fmt.Errorf("failed to load configuration from environment: %w", err)
	}

	cfg.ServiceName = os.Getenv("AWS_LAMBDA_FUNCTION_NAME")

	sdkConfig, err := config.LoadDefaultConfig(context.Background(), config.WithRetryer(func() aws.Retryer {
		// Adaptive mode should retry on hitting rate limits.
		return retry.AddWithMaxBackoffDelay(retry.NewAdaptiveMode(), 3*time.Second)
	}))
	if err != nil {
		return cfg, fmt.Errorf("failed to create AWS SDK configuration: %w", err)
	}

	ssmClient := client.NewSSM(&sdkConfig)

	lambdaClient := extension.NewLambda()
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
