package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/structs"
)

func TestAWSEventToEvents(t *testing.T) {
	ctx := context.Background()
	arn := "arn:aws:lambda:us-east-1:111111111111:function:lambda-1234"
	s1 := UpsertEvent{
		Service: structs.Service{Name: "lambda-1234"},
		LambdaArguments: LambdaArguments{
			ARN:            arn,
			InvocationMode: asynchronousInvocationMode,
		},
	}
	s1WithAliases := UpsertEventPlusMeta{
		UpsertEvent:   s1,
		CreateService: true,
	}

	lambda := mockLambdaClient(s1WithAliases)
	env := mockEnvironment(lambda, nil)
	loadFixture := func(filename string) AWSEvent {
		d, err := os.ReadFile("./fixtures/" + filename + ".json")
		require.NoError(t, err)

		var e AWSEvent
		err = json.Unmarshal(d, &e)
		require.NoError(t, err)

		return e
	}

	cases := []string{"tag_resource", "untag_resource", "create_function"}

	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			ctx := context.Background()
			event := loadFixture(c)

			events, err := env.AWSEventToEvents(ctx, event)
			require.NoError(t, err)
			require.Len(t, events, 1)
			e, ok := events[0].(UpsertEvent)
			require.True(t, ok)
			require.Equal(t, e.Name, s1.Name)
			require.Equal(t, e.ARN, s1.ARN)
		})
	}

	t.Run("with an unsupported event name", func(t *testing.T) {
		event := loadFixture("unsupported")
		_, err := env.AWSEventToEvents(ctx, event)
		require.Error(t, err)
	})
}

func TestGetLambdaData(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:111111111111:function:lambda-1234"
	makeService := func(enterprise bool, alias, dc string) UpsertEvent {
		e := UpsertEvent{
			LambdaArguments: LambdaArguments{
				ARN:                arn,
				PayloadPassthrough: true,
				InvocationMode:     asynchronousInvocationMode,
			},
			Service: structs.Service{Name: "lambda-1234", Datacenter: dc},
		}
		if enterprise {
			e.EnterpriseMeta = &structs.EnterpriseMeta{Namespace: "n", Partition: "p"}
		}
		if alias != "" {
			e.Name = e.Name + "-" + alias
			e.ARN = e.ARN + ":" + alias
		}
		return e
	}
	disabledService := makeService(false, "", "")
	serviceWithInvalidInvocationMode := makeService(false, "", "")
	serviceWithInvalidInvocationMode.InvocationMode = "invalid"

	cases := map[string]struct {
		arn          string
		upsertEvents []UpsertEventPlusMeta
		expected     []Event
		expectErr    bool
		err          bool
		enterprise   bool
		partitions   []string
		datacenter   string
	}{
		"Invalid arn": {
			arn:       "1234",
			expectErr: true,
		},
		"Enterprise meta is passed without enterprise Consul": {
			arn: arn,
			err: true,
			upsertEvents: []UpsertEventPlusMeta{
				{
					UpsertEvent: makeService(true, "", ""),
				},
			},
		},
		"Everything is passed - Enterprise": {
			arn: arn,
			err: false,
			upsertEvents: []UpsertEventPlusMeta{
				{
					UpsertEvent:   makeService(true, "", ""),
					CreateService: true,
				},
			},
			enterprise: true,
			expected: []Event{
				makeService(true, "", ""),
			},
			partitions: []string{"p"},
		},
		"Ignoring unhandled partitions - Enterprise": {
			arn: arn,
			err: false,
			upsertEvents: []UpsertEventPlusMeta{
				{
					UpsertEvent: makeService(true, "", ""),
				},
			},
			enterprise: true,
			partitions: []string{},
		},
		"Ignoring unhandled datacenter": {
			arn: arn,
			err: false,
			upsertEvents: []UpsertEventPlusMeta{
				{
					UpsertEvent: makeService(false, "", "dc2"),
				},
			},
			enterprise: false,
			datacenter: "dc1",
		},
		"Removing disabled services": {
			arn: arn,
			err: false,
			upsertEvents: []UpsertEventPlusMeta{
				{
					UpsertEvent:   disabledService,
					CreateService: false,
				},
			},
			enterprise: false,
			expected:   []Event{DeleteEvent{structs.Service{EnterpriseMeta: nil, Name: "lambda-1234"}}},
		},
		"Everything is passed - OSS": {
			arn: arn,
			err: false,
			upsertEvents: []UpsertEventPlusMeta{
				{
					UpsertEvent:   makeService(false, "", ""),
					CreateService: true,
				},
			},
			enterprise: false,
			expected: []Event{
				makeService(false, "", ""),
			},
		},
		"Invalid invocation mode": {
			arn: arn,
			err: true,
			upsertEvents: []UpsertEventPlusMeta{
				{
					UpsertEvent: serviceWithInvalidInvocationMode,
				},
			},
			enterprise: false,
		},
		"Aliases": {
			arn: arn,
			err: false,
			upsertEvents: []UpsertEventPlusMeta{
				{
					UpsertEvent:   makeService(false, "", ""),
					Aliases:       []string{"a1", "a2"},
					CreateService: true,
				},
			},
			enterprise: false,
			expected: []Event{
				makeService(false, "", ""),
				makeService(false, "a1", ""),
				makeService(false, "a2", ""),
			},
		},
	}

	for n, c := range cases {
		t.Run(n, func(t *testing.T) {
			ctx := context.Background()
			lambda := mockLambdaClient(c.upsertEvents...)
			env := mockEnvironment(lambda, nil)
			env.Datacenter = c.datacenter
			env.IsEnterprise = c.enterprise
			for _, p := range c.partitions {
				env.Partitions[p] = struct{}{}
			}

			fn, err := lambda.GetFunction(ctx, arn)
			if c.expectErr {
				require.Error(t, err)
				return
			} else {
				require.NoError(t, err)
			}

			events, err := env.GetLambdaEvents(fn)
			if c.err {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, c.expected, events)
		})
	}
}

func TestFullSyncData(t *testing.T) {
	enterprise := enterpriseFlag()

	var enterpriseMeta *structs.EnterpriseMeta
	if enterprise {
		enterpriseMeta = &structs.EnterpriseMeta{
			Namespace: "ns1",
			Partition: "ap1",
		}
	}

	s1 := UpsertEvent{
		Service: structs.Service{Name: "lambda-1234", EnterpriseMeta: enterpriseMeta},
		LambdaArguments: LambdaArguments{
			ARN:            "arn:aws:lambda:us-east-1:111111111111:function:lambda-1234",
			InvocationMode: "SYNCHRONOUS",
		},
	}
	service1 := UpsertEventPlusMeta{
		UpsertEvent:   s1,
		CreateService: true,
	}
	disabledService1 := service1
	disabledService1.CreateService = false

	s1prod := UpsertEvent{
		Service: structs.Service{Name: "lambda-1234-prod", EnterpriseMeta: enterpriseMeta},
		LambdaArguments: LambdaArguments{
			ARN:            s1.ARN + ":prod",
			InvocationMode: "SYNCHRONOUS",
		},
	}
	s1dev := UpsertEvent{
		Service: structs.Service{Name: "lambda-1234-dev", EnterpriseMeta: enterpriseMeta},
		LambdaArguments: LambdaArguments{
			ARN:            s1.ARN + ":dev",
			InvocationMode: "SYNCHRONOUS",
		},
	}
	service1WithAlias := UpsertEventPlusMeta{
		UpsertEvent:   s1,
		Aliases:       []string{"prod", "dev"},
		CreateService: true,
	}

	otherDCService1 := service1
	otherDCService1.Datacenter = "dc2"

	type caseData struct {
		// Set up consul state
		SeedConsulState []UpsertEventPlusMeta
		// Set up Lambda state
		SeedLambdaState []UpsertEventPlusMeta
		ExpectedEvents  []Event
		Partitions      []string
		Datacenter      string
	}

	cases := map[string]*caseData{
		"Add a service": {
			SeedLambdaState: []UpsertEventPlusMeta{service1},
			ExpectedEvents:  []Event{s1},
		},
		"Remove a service": {
			SeedConsulState: []UpsertEventPlusMeta{service1},
			ExpectedEvents: []Event{
				DeleteEvent{structs.Service{
					Name:           "lambda-1234",
					EnterpriseMeta: enterpriseMeta,
				}}},
		},
		"Ignore Lambdas without create service meta": {
			SeedLambdaState: []UpsertEventPlusMeta{disabledService1},
			ExpectedEvents:  []Event{},
		},
		"Ignore Lambdas in other datacenters": {
			SeedLambdaState: []UpsertEventPlusMeta{otherDCService1},
			ExpectedEvents:  []Event{},
			Datacenter:      "dc1",
		},
		"With aliases": {
			SeedLambdaState: []UpsertEventPlusMeta{service1WithAlias},
			ExpectedEvents:  []Event{s1, s1dev, s1prod},
		},
	}

	if enterprise {
		for k := range cases {
			cases[k].Partitions = []string{"default", "ap1"}
		}

		cases["Ignoring Lambdas in unhandled partitions"] = &caseData{
			SeedLambdaState: []UpsertEventPlusMeta{service1},
			ExpectedEvents:  []Event{},
			Partitions:      []string{},
		}

		cases["Ignoring Lambda Consul services in unhandled partitions"] = &caseData{
			SeedConsulState: []UpsertEventPlusMeta{service1},
			ExpectedEvents:  []Event{},
			Partitions:      []string{},
		}
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			c := c
			server, err := testutil.NewTestServerConfigT(t, nil)
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = server.Stop()
			})

			consulClient, err := api.NewClient(&api.Config{Address: server.HTTPAddr})
			require.NoError(t, err)

			if enterprise {
				_, _, err = consulClient.Partitions().Create(context.Background(), &api.Partition{Name: "ap1"}, nil)
				require.NoError(t, err)
				_, _, err = consulClient.Namespaces().Create(&api.Namespace{Name: "ns1", Partition: "ap1"}, &api.WriteOptions{
					Partition: "ap1",
				})
				require.NoError(t, err)
			}

			env := mockEnvironment(mockLambdaClient(c.SeedLambdaState...), consulClient)
			env.Datacenter = c.Datacenter
			env.IsEnterprise = enterprise
			for _, p := range c.Partitions {
				env.Partitions[p] = struct{}{}
			}

			for _, e := range c.SeedConsulState {
				err := e.Reconcile(env)
				require.NoError(t, err)
			}

			events, err := env.FullSyncData(ctx)
			require.NoError(t, err)
			require.ElementsMatch(t, c.ExpectedEvents, events)
		})
	}
}
