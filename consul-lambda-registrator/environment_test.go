package main

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdaTypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmTypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
)

func TestSetupEnvironment(t *testing.T) {
	envVars := map[string]string{
		nodeNameEnvironment:   "lambdas",
		awsRegionEnvironment:  "region",
		enterpriseEnvironment: "false",
		partitionsEnvironment: "a,b",
		logLevelEnvironment:   "warn",
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

	require.NotNil(t, env.Lambda)
	require.Equal(t, "lambdas", env.NodeName)
	require.NotNil(t, env.ConsulClient)
	require.Equal(t, "region", env.Region)
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
		err := setConsulCACert(ctx, ssmClient)
		require.NoError(t, err)
		_, err = os.Stat(caCertPath)
		require.Error(t, err)
		require.True(t, errors.Is(err, os.ErrNotExist))
	})

	t.Run("With a path that isn't in parameter store", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		os.Setenv(consulCAPathEnvironment, "not/real")
		ssmClient := mockSSMClient(map[string]string{})
		err := setConsulCACert(ctx, ssmClient)
		require.Error(t, err)
		_, err = os.Stat(caCertPath)
		require.Error(t, err)
		require.True(t, errors.Is(err, os.ErrNotExist))
	})

	t.Run("With a path is in parameter store", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		os.Setenv(consulCAPathEnvironment, "real")
		ssmClient := mockSSMClient(map[string]string{"real": "value"})
		err := setConsulCACert(ctx, ssmClient)
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
		err := setConsulHTTPToken(ctx, ssmClient)
		require.NoError(t, err)
		require.Equal(t, "", os.Getenv("CONSUL_HTTP_TOKEN"))
	})

	t.Run("With a path that isn't in parameter store", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		os.Setenv(consulHTTPTokenPath, "not/real")
		ssmClient := mockSSMClient(map[string]string{})
		err := setConsulHTTPToken(ctx, ssmClient)
		require.Error(t, err)
		require.Equal(t, "", os.Getenv("CONSUL_HTTP_TOKEN"))
	})

	t.Run("With a path is in parameter store", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		os.Setenv(consulHTTPTokenPath, "real")
		ssmClient := mockSSMClient(map[string]string{"real": "value"})
		err := setConsulHTTPToken(ctx, ssmClient)
		require.NoError(t, err)
		require.Equal(t, "value", os.Getenv("CONSUL_HTTP_TOKEN"))
	})
}

func mockSSMClient(mappings map[string]string) GetParameterAPIClient {
	return mockSSM{mappings: mappings}
}

type mockSSM struct {
	mappings map[string]string
}

var _ GetParameterAPIClient = (*mockSSM)(nil)

func (s mockSSM) GetParameter(_ context.Context, i *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	value := s.mappings[*i.Name]

	if value == "" {
		return nil, errors.New("Not found")
	}

	o := &ssm.GetParameterOutput{
		Parameter: &ssmTypes.Parameter{Value: &value},
	}

	return o, nil
}

func mockEnvironment(lambdaClient LambdaAPIClient, consulClient *api.Client) Environment {
	return Environment{
		Lambda:       lambdaClient,
		NodeName:     "lambdas",
		ConsulClient: consulClient,
		Region:       "us-east-1",
		IsEnterprise: false,
		Partitions:   make(map[string]struct{}),
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

func mockLambdaClient(events ...UpsertEventPlusMeta) LambdaClient {
	functions := make(map[string]*lambda.GetFunctionOutput)
	for _, event := range events {
		l := &lambda.GetFunctionOutput{
			Configuration: &lambdaTypes.FunctionConfiguration{
				FunctionName: &event.ServiceName,
				FunctionArn:  &event.ARN,
			},
			Tags: map[string]string{
				enabledTag:            strconv.FormatBool(event.CreateService),
				payloadPassthroughTag: strconv.FormatBool(event.PayloadPassthrough),
				invocationModeTag:     event.InvocationMode,
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

	return LambdaClient{
		Functions: functions,
	}
}

type LambdaClient struct {
	Functions map[string]*lambda.GetFunctionOutput
}

var _ LambdaAPIClient = (*LambdaClient)(nil)

func (lc LambdaClient) ListFunctions(_ context.Context, i *lambda.ListFunctionsInput, _ ...func(*lambda.Options)) (*lambda.ListFunctionsOutput, error) {
	var fns []lambdaTypes.FunctionConfiguration
	for _, v := range lc.Functions {
		fns = append(fns, *v.Configuration)
	}

	return &lambda.ListFunctionsOutput{Functions: fns}, nil
}

func (lc LambdaClient) GetFunction(_ context.Context, i *lambda.GetFunctionInput, _ ...func(*lambda.Options)) (*lambda.GetFunctionOutput, error) {
	fn := lc.Functions[*i.FunctionName]
	if fn == nil {
		return nil, errors.New("no tags")
	}
	return fn, nil
}
