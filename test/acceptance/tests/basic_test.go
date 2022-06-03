package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	terratestLogger "github.com/gruntwork-io/terratest/modules/logger"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/shell"
	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil/retry"
	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/test/acceptance/framework/config"
	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/test/acceptance/framework/logger"
	"github.com/stretchr/testify/require"
)

func TestBasic(t *testing.T) {
	cases := map[string]struct {
		secure     bool
		enterprise bool
	}{
		"secure": {
			secure: true,
		},
		"insecure": {
			secure: false,
		},
		"enterprise and secure": {
			secure:     true,
			enterprise: true,
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			config := suite.Config()
			tfVars := config.TFVars()
			tfVars["secure"] = c.secure
			namespace := ""
			partition := ""
			queryString := ""
			tfVars["consul_image"] = "public.ecr.aws/hashicorp/consul:1.12.1"

			if c.enterprise {
				tfVars["consul_license"] = os.Getenv("CONSUL_LICENSE")
				namespace = "ns1"
				partition = "ap1"
				tfVars["consul_namespace"] = namespace
				tfVars["consul_partition"] = partition
				queryString = fmt.Sprintf("?partition=%s&ns=%s", partition, namespace)
				tfVars["consul_image"] = "public.ecr.aws/hashicorp/consul-enterprise:1.12.1-ent"
			}

			setupSuffix := tfVars["suffix"]
			suffix := strings.ToLower(random.UniqueId())
			tfVars["suffix"] = suffix
			tfVars["setup_suffix"] = setupSuffix

			setupTerraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
				TerraformDir: "./setup",
				Vars:         tfVars,
				NoColor:      true,
			})

			lambdaTerraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
				TerraformDir: "./lambda",
				NoColor:      true,
			})

			t.Cleanup(func() {
				if suite.Config().NoCleanupOnFailure && t.Failed() {
					logger.Log(t, "skipping resource cleanup because -no-cleanup-on-failure=true")
				} else {
					terraform.Destroy(t, setupTerraformOptions)
					terraform.Destroy(t, lambdaTerraformOptions)
				}
			})

			terraform.InitAndApply(t, setupTerraformOptions)

			clientServiceName := fmt.Sprintf("test_client_%s", suffix)
			preexistingLambdaServiceName := fmt.Sprintf("preexisting_%s", setupSuffix)
			lambdaServiceName := fmt.Sprintf("example_%s", suffix)
			prodLambdaServiceName := fmt.Sprintf("example_%s-prod", suffix)
			devLambdaServiceName := fmt.Sprintf("example_%s-dev", suffix)

			var consulServerTaskARN string
			retry.RunWith(&retry.Timer{Timeout: 3 * time.Minute, Wait: 10 * time.Second}, t, func(r *retry.R) {
				taskListOut, err := shell.RunCommandAndGetOutputE(t, shell.Command{
					Command: "aws",
					Args: []string{
						"ecs",
						"list-tasks",
						"--region",
						suite.Config().Region,
						"--cluster",
						suite.Config().ECSClusterARN,
						"--family",
						fmt.Sprintf("lr-%s-consul-server", suffix),
					},
				})
				r.Check(err)

				var tasks listTasksResponse
				r.Check(json.Unmarshal([]byte(taskListOut), &tasks))
				if len(tasks.TaskARNs) != 1 {
					r.Errorf("expected 1 task, got %d", len(tasks.TaskARNs))
					return
				}

				consulServerTaskARN = tasks.TaskARNs[0]
			})

			// Wait for passing health check for test_server and test_client
			tokenHeader := ""
			if c.secure {
				tokenHeader = `-H "X-Consul-Token: $CONSUL_HTTP_TOKEN"`
			}

			var clientTaskARN string
			retry.RunWith(&retry.Timer{Timeout: 5 * time.Minute, Wait: 10 * time.Second}, t, func(r *retry.R) {
				taskListOut, err := shell.RunCommandAndGetOutputE(t, shell.Command{
					Command: "aws",
					Args: []string{
						"ecs",
						"list-tasks",
						"--region",
						suite.Config().Region,
						"--cluster",
						suite.Config().ECSClusterARN,
						"--family",
						clientServiceName,
					},
				})
				r.Check(err)

				var tasks listTasksResponse
				r.Check(json.Unmarshal([]byte(taskListOut), &tasks))
				if len(tasks.TaskARNs) != 1 {
					r.Errorf("expected 1 task, got %d", len(tasks.TaskARNs))
					return
				}

				clientTaskARN = tasks.TaskARNs[0]

				var services []api.CatalogService
				err = ExecuteRemoteCommandJSON(
					t,
					suite.Config(),
					consulServerTaskARN,
					"consul-server",
					fmt.Sprintf(`/bin/sh -c 'curl %s "localhost:8500/v1/catalog/service/%s%s"'`, tokenHeader, clientServiceName, queryString),
					&services,
				)
				r.Check(err)
				require.Len(r, services, 1)
			})

			tags := map[string]string{
				"serverless.consul.hashicorp.com/v1alpha1/lambda/enabled":              "true",
				"serverless.consul.hashicorp.com/v1alpha1/lambda/payload-passhthrough": "true",
				"serverless.consul.hashicorp.com/v1alpha1/lambda/aliases":              "prod+dev",
			}
			if c.enterprise {
				tags["serverless.consul.hashicorp.com/v1alpha1/lambda/partition"] = partition
				tags["serverless.consul.hashicorp.com/v1alpha1/lambda/namespace"] = namespace
			}

			lambdaTerraformOptions.Vars = map[string]interface{}{
				"tags":   tags,
				"name":   lambdaServiceName,
				"region": config.Region,
			}
			terraform.InitAndApply(t, lambdaTerraformOptions)

			lambdas := []struct {
				port               int
				name               string
				inDefaultPartition bool
			}{
				{
					port: 1234,
					name: lambdaServiceName,
				},
				{
					port: 1235,
					name: devLambdaServiceName,
				},
				{
					port: 1236,
					name: prodLambdaServiceName,
				},
				{
					port: 2345,
					name: preexistingLambdaServiceName,
					// This doesn't set up mesh gateways for cross-partition
					// communication.
					inDefaultPartition: c.enterprise,
				},
			}

			for _, c := range lambdas {
				retry.RunWith(&retry.Timer{Timeout: 60 * time.Second, Wait: 5 * time.Second}, t, func(r *retry.R) {
					var services []api.CatalogService
					qs := queryString
					if c.inDefaultPartition {
						qs = ""
					}
					err := ExecuteRemoteCommandJSON(
						t,
						suite.Config(),
						consulServerTaskARN,
						"consul-server",
						fmt.Sprintf(`/bin/sh -c 'curl %s "localhost:8500/v1/catalog/service/%s%s"'`, tokenHeader, c.name, qs),
						&services,
					)
					r.Check(err)
					require.Len(r, services, 1)

					if !c.inDefaultPartition {
						out, err := ExecuteRemoteCommand(
							t,
							suite.Config(),
							clientTaskARN,
							"basic",
							fmt.Sprintf(`curl -d "{\"name\": \"foo\"}" localhost:%d`, c.port),
						)
						expected := "Hello foo!"
						r.Check(err)
						require.Contains(r, out, expected)
					}
				})
			}

			lambdaTerraformOptions.Vars = map[string]interface{}{
				"tags": map[string]string{
					"serverless.consul.hashicorp.com/v1alpha1/lambda/enabled": "false",
				},
				"name":   lambdaServiceName,
				"region": config.Region,
			}
			terraform.InitAndApply(t, lambdaTerraformOptions)

			// Lambda doesn't exists
			retry.RunWith(&retry.Timer{Timeout: 60 * time.Second, Wait: 5 * time.Second}, t, func(r *retry.R) {
				var services []api.CatalogService
				err := ExecuteRemoteCommandJSON(
					t,
					suite.Config(),
					consulServerTaskARN,
					"consul-server",
					fmt.Sprintf(`/bin/sh -c 'curl %s "localhost:8500/v1/catalog/service/%s%s"'`, tokenHeader, lambdaServiceName, queryString),
					&services,
				)
				r.Check(err)
				require.Len(r, services, 0)
			})
		})
	}
}

// ExecuteRemoteCommand executes a command inside a container in the task specified
// by taskARN.
func ExecuteRemoteCommand(t *testing.T, testConfig *config.TestConfig, taskARN, container, command string) (string, error) {
	return shell.RunCommandAndGetOutputE(t, shell.Command{
		// TODO This uses unbuffer to get around an issue where `Cannot perform
		// start session: EOF` is added to the end of the response. This is
		// required because we parse the JSON in ExecuteRemoteCommandJSON.
		Command: "unbuffer",
		Args: []string{
			"aws",
			"ecs",
			"execute-command",
			"--region",
			testConfig.Region,
			"--cluster",
			testConfig.ECSClusterARN,
			"--task",
			taskARN,
			fmt.Sprintf("--container=%s", container),
			"--command",
			command,
			"--interactive",
		},
		Logger: terratestLogger.New(logger.TestLogger{}),
	})

}

func ExecuteRemoteCommandJSON(t *testing.T, testConfig *config.TestConfig, taskARN, container, command string, out interface{}) error {
	output, err := ExecuteRemoteCommand(t, testConfig, taskARN, container, command)
	if err != nil {
		return err
	}

	for _, line := range strings.Split(output, "\n") {
		err := json.Unmarshal([]byte(line), out)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("couldn't decode: %+v", output)
}

type listTasksResponse struct {
	TaskARNs []string `json:"taskArns"`
}
