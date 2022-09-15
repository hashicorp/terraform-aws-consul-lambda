package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
)

const (
	nodeNameEnvironment      string = "NODE_NAME"
	awsRegionEnvironment     string = "AWS_REGION"
	enterpriseEnvironment    string = "ENTERPRISE"
	datacenterEnvironment    string = "DATACENTER"
	partitionsEnvironment    string = "PARTITIONS"
	logLevelEnvironment      string = "LOG_LEVEL"
	consulCAPathEnvironment  string = "CONSUL_CACERT_PATH"
	consulHTTPTokenPath      string = "CONSUL_HTTP_TOKEN_PATH"
	extensionPathEnvironment string = "EXTENSION_DATA_PATH"
)

func TestSetupEnvironment(t *testing.T) {
	envVars := map[string]string{
		nodeNameEnvironment:      "lambdas",
		awsRegionEnvironment:     "region",
		enterpriseEnvironment:    "false",
		datacenterEnvironment:    "dc1",
		partitionsEnvironment:    "a,b",
		logLevelEnvironment:      "warn",
		extensionPathEnvironment: "/path/to/data",
	}
	for k, v := range envVars {
		os.Setenv(k, v)
		t.Cleanup(func() { os.Unsetenv(k) })
	}

	server, err := testutil.NewTestServerConfigT(t, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = server.Stop()
	})

	ctx := context.Background()
	env, err := SetupEnvironment(ctx)
	require.NoError(t, err)

	require.Equal(t, envVars[nodeNameEnvironment], env.NodeName)
	require.Equal(t, envVars[awsRegionEnvironment], env.Region)
	require.Equal(t, envVars[datacenterEnvironment], env.Datacenter)
	require.Equal(t, envVars[logLevelEnvironment], env.LogLevel)
	require.Equal(t, envVars[extensionPathEnvironment], env.ExtensionDataPrefix)
	require.NotNil(t, env.Lambda)
	require.NotNil(t, env.ConsulClient)
	require.NotNil(t, env.Logger)
	require.False(t, env.IsEnterprise)
	require.Equal(t, map[string]struct{}{"a": {}, "b": {}}, env.Partitions)
}

func TestSetConsulCACert(t *testing.T) {
	ctx := context.Background()
	unsetEverything := func() {
		os.Unsetenv(consulCAPathEnvironment)
		os.Unsetenv("CONSUL_CACERT")
		os.Remove(caCertPath)
	}

	t.Run("Without the environment variable set", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		ssmClient := mockSSMClient(map[string]string{})
		err := setConsulCACert(ctx, ssmClient, consulCAPathEnvironment)
		require.NoError(t, err)
		_, err = os.Stat(caCertPath)
		require.Error(t, err)
		require.True(t, errors.Is(err, os.ErrNotExist))
	})

	t.Run("With a path that isn't in parameter store", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		os.Setenv(consulCAPathEnvironment, "not/real")
		ssmClient := mockSSMClient(map[string]string{})
		err := setConsulCACert(ctx, ssmClient, consulCAPathEnvironment)
		require.Error(t, err)
		_, err = os.Stat(caCertPath)
		require.Error(t, err)
		require.True(t, errors.Is(err, os.ErrNotExist))
	})

	t.Run("With a path is in parameter store", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		os.Setenv(consulCAPathEnvironment, "real")
		ssmClient := mockSSMClient(map[string]string{"real": "value"})
		err := setConsulCACert(ctx, ssmClient, consulCAPathEnvironment)
		require.NoError(t, err)
		buf, err := os.ReadFile(caCertPath)
		require.NoError(t, err)
		require.Equal(t, "value", string(buf))
	})
}

func TestSetConsulHTTPToken(t *testing.T) {
	ctx := context.Background()
	unsetEverything := func() {
		os.Unsetenv(consulHTTPTokenPath)
		os.Unsetenv("CONSUL_HTTP_TOKEN")
		os.Remove(caCertPath)
	}

	t.Run("Without the environment variable set", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		ssmClient := mockSSMClient(map[string]string{})
		err := setConsulHTTPToken(ctx, ssmClient, consulHTTPTokenPath)
		require.NoError(t, err)
		require.Equal(t, "", os.Getenv("CONSUL_HTTP_TOKEN"))
	})

	t.Run("With a path that isn't in parameter store", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		os.Setenv(consulHTTPTokenPath, "not/real")
		ssmClient := mockSSMClient(map[string]string{})
		err := setConsulHTTPToken(ctx, ssmClient, consulHTTPTokenPath)
		require.Error(t, err)
		require.Equal(t, "", os.Getenv("CONSUL_HTTP_TOKEN"))
	})

	t.Run("With a path is in parameter store", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		os.Setenv(consulHTTPTokenPath, "real")
		ssmClient := mockSSMClient(map[string]string{"real": "value"})
		err := setConsulHTTPToken(ctx, ssmClient, consulHTTPTokenPath)
		require.NoError(t, err)
		require.Equal(t, "value", os.Getenv("CONSUL_HTTP_TOKEN"))
	})
}

func mockSSMClient(mappings map[string]string) ParamStore {
	return mockSSM{mappings: mappings}
}

type mockSSM struct {
	mappings map[string]string
}

var _ ParamStore = (*mockSSM)(nil)

func (s mockSSM) Delete(_ context.Context, key string) error {
	delete(s.mappings, key)
	return nil
}

func (s mockSSM) Get(_ context.Context, key string) (string, error) {
	if value, ok := s.mappings[key]; ok && value != "" {
		return value, nil
	}
	return "", fmt.Errorf("unable to get %s: not found", key)
}

func (s mockSSM) Set(_ context.Context, key, val string) error {
	s.mappings[key] = val
	return nil
}

func mockEnvironment(lambdaClient LambdaAPIClient, consulClient *api.Client) Environment {
	return Environment{
		Config: Config{
			NodeName:     "lambdas",
			Region:       "us-east-1",
			IsEnterprise: false,
			Partitions:   make(map[string]struct{}),
		},
		Lambda:       lambdaClient,
		ConsulClient: consulClient,
		Logger: hclog.New(
			&hclog.LoggerOptions{
				Level: hclog.LevelFromString("info"),
			},
		),
	}
}

type UpsertEventPlusMeta struct {
	UpsertEvent
	Aliases       []string
	CreateService bool
}

func mockLambdaClient(events ...UpsertEventPlusMeta) mockLambda {
	functions := make(map[string]LambdaFunction, len(events))
	for _, event := range events {
		l := LambdaFunction{
			ARN:  event.ARN,
			Name: event.Name,
			Tags: map[string]string{
				enabledTag:            strconv.FormatBool(event.CreateService),
				payloadPassthroughTag: strconv.FormatBool(event.PayloadPassthrough),
				invocationModeTag:     event.InvocationMode,
				datacenterTag:         event.Datacenter,
			},
		}

		if len(event.Aliases) > 0 {
			l.Tags[aliasesTag] = strings.Join(event.Aliases, listSeparator)
		}

		if em := event.EnterpriseMeta; em != nil {
			l.Tags[namespaceTag] = em.Namespace
			l.Tags[partitionTag] = em.Partition
		}

		functions[event.ARN] = l
	}

	return mockLambda{
		Functions: functions,
	}
}

type mockLambda struct {
	Functions map[string]LambdaFunction
}

var _ LambdaAPIClient = (*mockLambda)(nil)

func (lc mockLambda) ListFunctions(_ context.Context) (map[string]LambdaFunction, error) {
	return lc.Functions, nil
}

func (lc mockLambda) GetFunction(_ context.Context, arn string) (LambdaFunction, error) {
	if fn, ok := lc.Functions[arn]; ok {
		return fn, nil
	}
	return LambdaFunction{}, fmt.Errorf("function %s does not exist", arn)
}
