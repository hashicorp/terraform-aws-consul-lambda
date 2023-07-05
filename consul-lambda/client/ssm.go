// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// SSMClient provides an API client for interacting with AWS Systems Manager Parameter Store.
type SSMClient struct {
	client *ssm.Client
	tier   types.ParameterTier
}

// NewSSM creates an instance of the SSMClient from the given AWS SDK config.
func NewSSM(cfg *aws.Config, aTier bool) *SSMClient {
	var tier = types.ParameterTierStandard

	if aTier {
		tier = types.ParameterTierAdvanced
	}

	return &SSMClient{client: ssm.NewFromConfig(*cfg), tier: tier}
}

// Delete removes the value for the given key from Parameter Store.
func (c *SSMClient) Delete(ctx context.Context, key string) error {
	_, err := c.client.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: &key})
	return err
}

// Get retrieves the value for the given key from Parameter Store.
// Get assumes that the value is encrypted as a SecureString and returns the decrypted value.
func (c *SSMClient) Get(ctx context.Context, key string) (string, error) {
	paramValue, err := c.client.GetParameter(
		ctx,
		&ssm.GetParameterInput{
			Name:           &key,
			WithDecryption: true,
		})

	if err != nil {
		return "", err
	}

	val := paramValue.Parameter.Value
	if val == nil {
		return "", fmt.Errorf("parameter store value does not exist for %s", key)
	}
	return *val, nil
}

// Set writes the value for the given key to Parameter Store.
// It writes the value as an encrypted SecureString.
// Any existing data for the given key is overwritten.
func (c *SSMClient) Set(ctx context.Context, key, val string) error {
	if c.tier == types.ParameterTierAdvanced {
		if len(val) < 4097 {
			c.tier = types.ParameterTierStandard
		}
	}

	input := &ssm.PutParameterInput{
		Name:      &key,
		Value:     &val,
		Overwrite: true,
		Type:      types.ParameterTypeSecureString,
		Tier:      c.tier,
	}

	_, err := c.client.PutParameter(ctx, input)
	return err
}
