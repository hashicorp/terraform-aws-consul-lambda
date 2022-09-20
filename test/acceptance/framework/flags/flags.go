package flags

import (
	"encoding/json"
	"flag"
	"fmt"
	"os/exec"
	"sync"

	"github.com/hashicorp/terraform-aws-consul-lambda/test/acceptance/framework/config"
)

const (
	flagNoCleanupOnFailure = "no-cleanup-on-failure"
	flagECSClusterARN      = "ecs-cluster-arn"
	flagPrivateSubnets     = "private_subnets"
	flagPublicSubnets      = "public-subnets"
	flagRegion             = "region"
	flagLogGroupName       = "log-group-name"
	flagTFOutputDir        = "tf-output-dir"
	flagVPCID              = "vpc-id"
	flagECRImageURI        = "ecr-image-uri"
	flagSuffix             = "suffix"

	setupTerraformDir = "../setup-terraform"
)

type TestFlags struct {
	flagNoCleanupOnFailure bool
	flagECSClusterARN      string
	flagPrivateSubnets     string
	flagPublicSubnets      string
	flagRegion             string
	flagLogGroupName       string
	flagTFOutputDir        string
	flagVPCID              string
	flagECRImageURI        string
	flagSuffix             string

	once sync.Once
}

func NewTestFlags() *TestFlags {
	t := &TestFlags{}
	t.once.Do(t.init)

	return t
}

func (t *TestFlags) init() {
	flag.BoolVar(&t.flagNoCleanupOnFailure, flagNoCleanupOnFailure, false,
		"If true, the tests will not clean up resources they create when they finish running."+
			"Note this flag must be run with -failfast flag, otherwise subsequent tests will fail.")
	flag.StringVar(&t.flagECSClusterARN, flagECSClusterARN, "", "ECS Cluster ARN.")
	flag.StringVar(&t.flagPrivateSubnets, flagPrivateSubnets, "", "Private subnets to deploy into. In TF var form, e.g. '[\"sub1\",\"sub2\"]'.")
	flag.StringVar(&t.flagPublicSubnets, flagPublicSubnets, "", "Public subnets to deploy into. In TF var form, e.g. '[\"sub1\",\"sub2\"]'.")
	flag.StringVar(&t.flagVPCID, flagVPCID, "", "VPC to deploy into.")
	flag.StringVar(&t.flagRegion, flagRegion, "", "AWS Region.")
	flag.StringVar(&t.flagLogGroupName, flagLogGroupName, "", "CloudWatch log group name.")
	flag.StringVar(&t.flagECRImageURI, flagECRImageURI, "", "Lambda registrator's container image.")
	flag.StringVar(&t.flagTFOutputDir, flagTFOutputDir, setupTerraformDir, "The directory of the setup terraform state for the tests.")
	flag.StringVar(&t.flagSuffix, flagSuffix, setupTerraformDir, "The suffix to use when naming resources.")
}

func (t *TestFlags) Validate() error {
	// todo: require certain vars
	return nil
}

type tfOutputItem struct {
	Value interface{}
	Type  interface{}
}

func (t *TestFlags) TestConfigFromFlags() (*config.TestConfig, error) {
	var cfg config.TestConfig

	// If there is a terraform output directory, use that to create test config.
	if t.flagTFOutputDir != "" {
		// We use tfOutput to parse the terraform output.
		// We then read the parsed output and put into tfOutputValues,
		// extracting only Values from the output.
		var tfOutput map[string]tfOutputItem
		tfOutputValues := make(map[string]interface{})

		// Get terraform output as JSON.
		cmd := exec.Command("terraform", "output", "-state", fmt.Sprintf("%s/terraform.tfstate", t.flagTFOutputDir), "-json")
		cmdOutput, err := cmd.CombinedOutput()
		if err != nil {
			return nil, err
		}

		// Parse terraform output into tfOutput map.
		err = json.Unmarshal(cmdOutput, &tfOutput)
		if err != nil {
			return nil, err
		}

		// Extract Values from the parsed output into a separate map.
		for k, v := range tfOutput {
			tfOutputValues[k] = v.Value
		}

		// Marshal the resulting map back into JSON so that
		// we can unmarshal it into the TestConfig struct directly.
		testConfigJSON, err := json.Marshal(tfOutputValues)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(testConfigJSON, &cfg)
		if err != nil {
			return nil, err
		}
	} else {
		cfg = config.TestConfig{
			NoCleanupOnFailure: t.flagNoCleanupOnFailure,
			ECSClusterARN:      t.flagECSClusterARN,
			PrivateSubnets:     t.flagPrivateSubnets,
			PublicSubnets:      t.flagPublicSubnets,
			Region:             t.flagRegion,
			LogGroupName:       t.flagLogGroupName,
			ECRImageURI:        t.flagECRImageURI,
			VPCID:              t.flagVPCID,
		}
	}

	cfg.NoCleanupOnFailure = t.flagNoCleanupOnFailure

	return &cfg, nil
}
