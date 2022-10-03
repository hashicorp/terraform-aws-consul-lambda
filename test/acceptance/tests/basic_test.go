package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	terratestLogger "github.com/gruntwork-io/terratest/modules/logger"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/shell"
	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil/retry"
	"github.com/hashicorp/terraform-aws-consul-lambda/test/acceptance/framework/config"
	"github.com/hashicorp/terraform-aws-consul-lambda/test/acceptance/framework/logger"
	"github.com/stretchr/testify/require"
)

type SetupConfig struct {
	MeshGatewayURI string `json:"mesh_gateway_uri"`
}

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
				require.NotEmpty(t, tfVars["consul_license"], "CONSUL_LICENSE environment variable is required for enterprise tests")
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

			t.Cleanup(func() {
				if suite.Config().NoCleanupOnFailure && t.Failed() {
					logger.Log(t, "skipping resource cleanup for ./setup because -no-cleanup-on-failure=true")
				} else {
					terraform.Destroy(t, setupTerraformOptions)
				}
			})

			terraform.InitAndApply(t, setupTerraformOptions)

			var setupCfg SetupConfig
			require.NoError(t, UnmarshalTF("./setup", &setupCfg))

			clientServiceName := fmt.Sprintf("test_client_%s", suffix)
			preexistingLambdaServiceName := fmt.Sprintf("preexisting_%s", setupSuffix)
			mtlServiceName := fmt.Sprintf("mtl_example_%s", suffix)
			prodLambdaServiceName := fmt.Sprintf("mtl_example_%s-prod", suffix)
			devLambdaServiceName := fmt.Sprintf("mtl_example_%s-dev", suffix)
			ltmServiceName := fmt.Sprintf("ltm_example_%s", suffix)

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

			// Wait for passing health check for test_client
			tokenHeader := ""
			if c.secure {
				tokenHeader = `-H "X-Consul-Token: $CONSUL_HTTP_TOKEN"`
			}

			retry.RunWith(&retry.Timer{Timeout: 5 * time.Minute, Wait: 10 * time.Second}, t, func(r *retry.R) {
				var services []api.CatalogService
				err := ExecuteRemoteCommandJSON(
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

			// Create Lambda functions that are called by the test_client
			tags := map[string]string{
				"serverless.consul.hashicorp.com/v1alpha1/lambda/enabled":             "true",
				"serverless.consul.hashicorp.com/v1alpha1/lambda/payload-passthrough": "true",
				"serverless.consul.hashicorp.com/v1alpha1/lambda/aliases":             "prod+dev",
			}
			if c.enterprise {
				tags["serverless.consul.hashicorp.com/v1alpha1/lambda/partition"] = partition
				tags["serverless.consul.hashicorp.com/v1alpha1/lambda/namespace"] = namespace
			}

			mtlTerraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
				TerraformDir: "./mtl",
				NoColor:      true,
				Vars: map[string]interface{}{
					"tags":   tags,
					"name":   mtlServiceName,
					"region": config.Region,
				},
			})

			t.Cleanup(func() {
				if suite.Config().NoCleanupOnFailure && t.Failed() {
					logger.Log(t, "skipping resource cleanup for ./mtl because -no-cleanup-on-failure=true")
				} else {
					terraform.Destroy(t, mtlTerraformOptions)
				}
			})

			terraform.InitAndApply(t, mtlTerraformOptions)

			// Create Lambda function that calls the test_client
			ltmAP := ""
			ltmNS := ""
			env := map[string]string{
				"CONSUL_EXTENSION_DATA_PREFIX": "/" + suffix,
				"CONSUL_MESH_GATEWAY_URI":      setupCfg.MeshGatewayURI,
				"CONSUL_SERVICE_UPSTREAMS":     clientServiceName + ":1234",

				// These vars configure the function itself.
				"UPSTREAMS":     "http://localhost:1234",
				"TRACE_ENABLED": "true",
				"LOG_LEVEL":     "debug",
			}
			if c.enterprise {
				env["CONSUL_SERVICE_UPSTREAMS"] = fmt.Sprintf("%s.%s.%s:1234", clientServiceName, namespace, partition)

				// Lambda functions don't have a Consul client agent so Lambda registrator uses the HTTP API
				// to retrieve the leaf service certificate for the Lambda function. That works in the default
				// partition and namespace but it does not work when the Lambda function is in a non-default
				// ap/ns. The registrator hits the /agent/connect/ca/leaf/:service endpoint on the Consul server
				// but supplying any non-default partition to this endpoint on the server results in:
				//
				//	request targets partition "ap1" which does not match agent partition "default"
				//
				// So we can't get a service cert for a Lambda function in a non-default partition :(
				ltmAP = "default"
				ltmNS = "default"
				env["CONSUL_SERVICE_PARTITION"] = ltmAP
				env["CONSUL_SERVICE_NAMESPACE"] = ltmNS
			}

			ltmTerraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
				TerraformDir: "./ltm",
				NoColor:      true,
				Vars: map[string]interface{}{
					"tags": map[string]string{
						"serverless.consul.hashicorp.com/v1alpha1/lambda/enabled": "true",
					},
					"name":   ltmServiceName,
					"region": config.Region,
					"env":    env,
					"layers": []string{suite.Config().ExtensionARN},
				},
			})

			t.Cleanup(func() {
				if suite.Config().NoCleanupOnFailure && t.Failed() {
					logger.Log(t, "skipping resource cleanup for ./ltm because -no-cleanup-on-failure=true")
				} else {
					terraform.Destroy(t, ltmTerraformOptions)
				}
			})

			terraform.InitAndApply(t, ltmTerraformOptions)

			//execCommand(t, suite.Config(), consulServerTaskARN, "consul-server", "cmd.txt", "eoc")

			lambdas := []struct {
				name               string
				inDefaultPartition bool
			}{
				{
					name: mtlServiceName,
				},
				{
					name: devLambdaServiceName,
				},
				{
					name: prodLambdaServiceName,
				},
				{
					name:               preexistingLambdaServiceName,
					inDefaultPartition: c.enterprise,
				},
				{
					name:               ltmServiceName,
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
				})
			}

			if c.secure {
				// Create an intention to allow the Lambda function to call the test_client service
				retry.RunWith(&retry.Timer{Timeout: 60 * time.Second, Wait: 5 * time.Second}, t, func(r *retry.R) {
					result, err := ExecuteRemoteCommand(
						t,
						suite.Config(),
						consulServerTaskARN,
						"consul-server",
						fmt.Sprintf(`/bin/sh -c 'curl %s -XPUT "localhost:8500/v1/config" -d"%s"'`,
							tokenHeader,
							buildIntention(ltmServiceName, ltmAP, ltmNS, clientServiceName, partition, namespace)),
					)
					r.Check(err)
					require.Contains(r, result, "true")
				})
			}

			if c.enterprise {
				// Export the test_client service so it can be called by the Lambda function.
				retry.RunWith(&retry.Timer{Timeout: 60 * time.Second, Wait: 5 * time.Second}, t, func(r *retry.R) {
					result, err := ExecuteRemoteCommand(
						t,
						suite.Config(),
						consulServerTaskARN,
						"consul-server",
						fmt.Sprintf(`/bin/sh -c 'curl %s -XPUT "localhost:8500/v1/config" -d"%s"'`,
							tokenHeader,
							buildExport(clientServiceName, partition, namespace, ltmAP)),
					)
					r.Check(err)
					require.Contains(r, result, "true")
				})
			}

			// Call the "Lambda to mesh" function. It is configured with the test_client as its upstream.
			// In turn, the test_client will invoke each of the other Lambda functions because they are configured
			// as upstreams of the test_client.
			// This way we cover both the Lambda-to-mesh and mesh-to-Lambda use cases in one call.
			outFile, err := os.CreateTemp("", "lambda-output")
			require.NoError(t, err)
			defer os.Remove(outFile.Name())

			retry.RunWith(&retry.Timer{Timeout: 120 * time.Second, Wait: 5 * time.Second}, t, func(r *retry.R) {
				_, err := shell.RunCommandAndGetOutputE(t, shell.Command{
					Command: "aws",
					Args: []string{
						"lambda",
						"invoke",
						"--region",
						suite.Config().Region,
						"--function-name",
						ltmServiceName,
						"--payload",
						base64.StdEncoding.EncodeToString([]byte(`{"lambda-to-mesh":true}`)),
						outFile.Name(),
					}},
				)
				r.Check(err)

				// The test_client will only return 200 when all upstreams return 200's.
				// Check for a 200 from the test_client response body.
				result, err := os.ReadFile(outFile.Name())
				r.Check(err)

				obs := struct {
					Body []struct {
						Body struct {
							Code int `json:"code"`
						} `json:"body"`
					} `json:"body"`
				}{}
				err = json.Unmarshal(result, &obs)
				r.Check(err)

				require.Len(r, obs.Body, 1)
				require.Equal(r, http.StatusOK, obs.Body[0].Body.Code)
			})

			mtlTerraformOptions.Vars = map[string]interface{}{
				"tags": map[string]string{
					"serverless.consul.hashicorp.com/v1alpha1/lambda/enabled": "false",
				},
				"name":   mtlServiceName,
				"region": config.Region,
			}
			terraform.InitAndApply(t, mtlTerraformOptions)

			// Lambda doesn't exists
			retry.RunWith(&retry.Timer{Timeout: 60 * time.Second, Wait: 5 * time.Second}, t, func(r *retry.R) {
				var services []api.CatalogService
				err := ExecuteRemoteCommandJSON(
					t,
					suite.Config(),
					consulServerTaskARN,
					"consul-server",
					fmt.Sprintf(`/bin/sh -c 'curl %s "localhost:8500/v1/catalog/service/%s%s"'`, tokenHeader, mtlServiceName, queryString),
					&services,
				)
				r.Check(err)
				require.Len(r, services, 0)
			})

			logger.Log(t, "Test successful!")
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

func buildIntention(src, srcAP, srcNS, dst, dstAP, dstNS string) string {
	var intention string
	if srcAP == "" && srcNS == "" && dstAP == "" && dstNS == "" {
		intention = fmt.Sprintf(`{"Kind":"service-intentions","Name":"%s","Sources":[{"Action":"allow","Name":"%s"}]}`,
			dst, src)
	} else {
		intention = fmt.Sprintf(`{"Kind":"service-intentions","Name":"%s","Partition":"%s","Namespace":"%s","Sources":[{"Action":"allow","Name":"%s","Partition":"%s","Namespace":"%s"}]}`,
			dst, dstAP, dstNS, src, srcAP, srcNS)
	}
	return strings.ReplaceAll(intention, `"`, `\"`)
}

func buildExport(dst, ap, ns, srcAP string) string {
	export := fmt.Sprintf(`{"Kind":"exported-services","Name":"%s","Partition":"%s","Services":[{"Name":"%s","Namespace":"%s","Consumers":[{"Partition":"%s"}]}]}`,
		ap, ap, dst, ns, srcAP)
	return strings.ReplaceAll(export, `"`, `\"`)
}

// UnmarshalTF populates the cfg struct with the Terraform outputs
// from the given tfDir directory. The cfg arg must be a pointer to
// a value that can be populated by json.Unmarshal based on the output
// of the `terraform output -json` command.
func UnmarshalTF(tfDir string, cfg interface{}) error {
	type tfOutputItem struct {
		Value interface{}
		Type  interface{}
	}
	// We use tfOutput to parse the terraform output.
	// We then read the parsed output and put into tfOutputValues,
	// extracting only Values from the output.
	var tfOutput map[string]tfOutputItem
	tfOutputValues := make(map[string]interface{})

	// Get terraform output as JSON.
	cmd := exec.Command("terraform", "output", "-state", fmt.Sprintf("%s/terraform.tfstate", tfDir), "-json")
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	// Parse terraform output into tfOutput map.
	err = json.Unmarshal(cmdOutput, &tfOutput)
	if err != nil {
		return err
	}

	// Extract Values from the parsed output into a separate map.
	for k, v := range tfOutput {
		tfOutputValues[k] = v.Value
	}

	// Marshal the resulting map back into JSON so that
	// we can unmarshal it into the target struct directly.
	configJSON, err := json.Marshal(tfOutputValues)
	if err != nil {
		return err
	}
	return json.Unmarshal(configJSON, cfg)
}

// TFVars converts the given struct to a map[string]interface that
// is suitable for supplying to terraform ... -var=...
// It iterates over the fields in the struct and creates a key for each field with a json tag.
//
// ignoreVars is optional and if provided any matching fields will be
// not be returned in the map.
//
// The argument i must be a struct value or a pointer to a struct; otherwise,
// the function will panic.
func TFVars(i interface{}, ignoreVars ...string) map[string]interface{} {
	v := reflect.ValueOf(i)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	if t.Kind() != reflect.Struct {
		panic("input must be a struct or pointer to a struct")
	}

	vars := make(map[string]interface{})
	structVars(i, vars)
	for _, v := range ignoreVars {
		delete(vars, v)
	}
	return vars
}

func structVars(i interface{}, m map[string]interface{}) {
	v := reflect.ValueOf(i)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type.Kind() == reflect.Ptr || f.Type.Kind() == reflect.Struct {
			// if the embedded field is a ptr or a struct recurse it
			structVars(v.Field(i).Interface(), m)
		} else {
			tag := t.Field(i).Tag.Get("json")
			if tag != "" {
				name := strings.Split(tag, ",")
				m[name[0]] = v.Field(i).Interface()
			}
		}
	}
}
