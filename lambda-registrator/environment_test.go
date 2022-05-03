package main

import (
	"errors"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/stretchr/testify/require"

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
