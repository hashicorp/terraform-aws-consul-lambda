// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/structs"
	"github.com/stretchr/testify/assert"
)

func TestUpsertEvent_Identifier(t *testing.T) {
	expectedARN := "0xda1@%@!%)?"
	e := UpsertEvent{
		LambdaArguments: LambdaArguments{
			ARN: expectedARN,
		},
	}

	assert.Equal(t, expectedARN, e.Identifier())
}

func TestWriteOptions(t *testing.T) {
	testCases := []struct {
		name     string
		input    structs.Service
		expected *api.WriteOptions
	}{
		{
			name: "no-enterprise-meta",
			input: structs.Service{
				Datacenter: "dc-1",
			},
			expected: &api.WriteOptions{
				Datacenter: "dc-1",
			},
		},
		{
			name: "with-enterprise-meta",
			input: structs.Service{
				Datacenter: "dc-1",
				EnterpriseMeta: &structs.EnterpriseMeta{
					Partition: "my-part",
					Namespace: "ns",
				},
			},
			expected: &api.WriteOptions{
				Datacenter: "dc-1",
				Partition:  "my-part",
				Namespace:  "ns",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := WriteOptions(tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
