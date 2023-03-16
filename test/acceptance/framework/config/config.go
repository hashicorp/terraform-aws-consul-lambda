// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package config

// TestConfig holds configuration for the test suite.
type TestConfig struct {
	NoCleanupOnFailure bool
	ECSClusterARN      string      `json:"ecs_cluster_arn"`
	PrivateSubnets     interface{} `json:"private_subnets"`
	PublicSubnets      interface{} `json:"public_subnets"`
	Region             string      `json:"region"`
	LogGroupName       string      `json:"log_group_name"`
	VPCID              string      `json:"vpc_id"`
	SecurityGroupID    string      `json:"security_group_id"`
	ECRImageURI        string      `json:"ecr_image_uri"`
	Suffix             string      `json:"suffix"`
	ExtensionARN       string      `json:"consul_lambda_extension_arn"`
}

func (t TestConfig) TFVars(ignoreVars ...string) map[string]interface{} {
	vars := map[string]interface{}{
		"ecs_cluster_arn":             t.ECSClusterARN,
		"private_subnets":             t.PrivateSubnets,
		"public_subnets":              t.PublicSubnets,
		"region":                      t.Region,
		"log_group_name":              t.LogGroupName,
		"vpc_id":                      t.VPCID,
		"security_group_id":           t.SecurityGroupID,
		"ecr_image_uri":               t.ECRImageURI,
		"suffix":                      t.Suffix,
		"consul_lambda_extension_arn": t.ExtensionARN,
	}

	for _, v := range ignoreVars {
		delete(vars, v)
	}
	return vars
}
