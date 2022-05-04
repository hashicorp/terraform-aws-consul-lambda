package main

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/lambda"
	"github.com/aws/aws-sdk-go/service/lambda/lambdaiface"
	"github.com/aws/aws-sdk-go/service/ssm"

	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
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

	env, err := SetupEnvironment()
	require.NoError(t, err)

	require.NotNil(t, env.Lambda)
	require.Equal(t, "lambdas", env.NodeName)
	require.NotNil(t, env.ConsulClient)
	require.Equal(t, "region", env.Region)
	require.False(t, env.IsEnterprise)
	require.Equal(t, []string{"a", "b"}, env.Partitions)
}

func TestSetConsulCACert(t *testing.T) {
	unsetEverything := func() {
		os.Unsetenv(consulCAPathEnvironment)
		os.Unsetenv("CONSUL_CACERT")
		os.Remove(caCertPath)
	}

	t.Run("Without the environment variable set", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		ssmClient := mockSSMClient(map[string]string{})
		err := setConsulCACert(ssmClient)
		require.NoError(t, err)
		_, err = os.Stat(caCertPath)
		require.Error(t, err)
		require.True(t, errors.Is(err, os.ErrNotExist))
	})

	t.Run("With a path that isn't in parameter store", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		os.Setenv(consulCAPathEnvironment, "not/real")
		ssmClient := mockSSMClient(map[string]string{})
		err := setConsulCACert(ssmClient)
		require.Error(t, err)
		_, err = os.Stat(caCertPath)
		require.Error(t, err)
		require.True(t, errors.Is(err, os.ErrNotExist))
	})

	t.Run("With a path is in parameter store", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		os.Setenv(consulCAPathEnvironment, "real")
		ssmClient := mockSSMClient(map[string]string{"real": "value"})
		err := setConsulCACert(ssmClient)
		require.NoError(t, err)
		buf, err := os.ReadFile(caCertPath)
		require.NoError(t, err)
		require.Equal(t, "value", string(buf))
	})
}

func TestSetConsulHTTPToken(t *testing.T) {
	unsetEverything := func() {
		os.Unsetenv(consulHTTPTokenPath)
		os.Unsetenv("CONSUL_HTTP_TOKEN")
		os.Remove(caCertPath)
	}

	t.Run("Without the environment variable set", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		ssmClient := mockSSMClient(map[string]string{})
		err := setConsulHTTPToken(ssmClient)
		require.NoError(t, err)
		require.Equal(t, "", os.Getenv("CONSUL_HTTP_TOKEN"))
	})

	t.Run("With a path that isn't in parameter store", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		os.Setenv(consulHTTPTokenPath, "not/real")
		ssmClient := mockSSMClient(map[string]string{})
		err := setConsulHTTPToken(ssmClient)
		require.Error(t, err)
		require.Equal(t, "", os.Getenv("CONSUL_HTTP_TOKEN"))
	})

	t.Run("With a path is in parameter store", func(t *testing.T) {
		t.Cleanup(unsetEverything)
		os.Setenv(consulHTTPTokenPath, "real")
		ssmClient := mockSSMClient(map[string]string{"real": "value"})
		err := setConsulHTTPToken(ssmClient)
		require.NoError(t, err)
		require.Equal(t, "value", os.Getenv("CONSUL_HTTP_TOKEN"))
	})
}

func mockSSMClient(mappings map[string]string) ssmiface.SSMAPI {
	return mockSSM{mappings: mappings}
}

type mockSSM struct {
	mappings map[string]string
	ssmiface.SSMAPI
}

func (s mockSSM) GetParameter(i *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
	value := s.mappings[*i.Name]

	if value == "" {
		return nil, errors.New("Not found")
	}

	o := &ssm.GetParameterOutput{
		Parameter: &ssm.Parameter{Value: &value},
	}

	return o, nil
}

func mockEnvironment(lambdaClient lambdaiface.LambdaAPI, consulClient *api.Client) Environment {
	return Environment{
		Lambda:       lambdaClient,
		NodeName:     "lambdas",
		ConsulClient: consulClient,
		Region:       "us-east-1",
		IsEnterprise: false,
		Partitions:   []string{},
		Logger: hclog.New(
			&hclog.LoggerOptions{
				Level: hclog.LevelFromString("info"),
			},
		),
	}
}

type UpsertEventPlusAliases struct {
	UpsertEvent
	Aliases []string
}

func mockLambdaClient(events ...UpsertEventPlusAliases) LambdaClient {
	functions := make(map[string]*lambda.GetFunctionOutput)
	tags := make(map[string]*lambda.ListTagsOutput)
	for _, event := range events {
		t := &lambda.ListTagsOutput{
			Tags: map[string]*string{
				enabledTag:            aws.String(strconv.FormatBool(event.CreateService)),
				payloadPassthroughTag: aws.String(strconv.FormatBool(event.PayloadPassthrough)),
			},
		}

		if len(event.Aliases) > 0 {
			t.Tags[aliasesTag] = aws.String(strings.Join(event.Aliases, ","))
		}

		if em := event.EnterpriseMeta; em != nil {
			t.Tags[namespaceTag] = &em.Namespace
			t.Tags[partitionTag] = &em.Partition
		}

		l := &lambda.GetFunctionOutput{
			Configuration: &lambda.FunctionConfiguration{
				FunctionName: &event.ServiceName,
				FunctionArn:  &event.ARN,
			},
		}

		functions[event.ARN] = l
		tags[event.ARN] = t
	}

	return LambdaClient{
		Functions: functions,
		Tags:      tags,
	}
}

type LambdaClient struct {
	lambdaiface.LambdaAPI
	Functions map[string]*lambda.GetFunctionOutput
	Tags      map[string]*lambda.ListTagsOutput
}

func (lc LambdaClient) ListFunctions(i *lambda.ListFunctionsInput) (*lambda.ListFunctionsOutput, error) {
	var fns []*lambda.FunctionConfiguration
	for _, v := range lc.Functions {
		fns = append(fns, v.Configuration)
	}

	return &lambda.ListFunctionsOutput{
		Functions: fns,
	}, nil
}

func (lc LambdaClient) ListTags(i *lambda.ListTagsInput) (*lambda.ListTagsOutput, error) {
	tags := lc.Tags[*i.Resource]
	if tags == nil {
		return nil, errors.New("no tags")
	}
	return tags, nil
}
