package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
)

func TestAWSEventToEvents(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:111111111111:function:lambda-1234"
	s1 := UpsertEvent{
		ServiceName:    "lambda-1234",
		ARN:            arn,
		InvocationMode: asynchronousInvocationMode,
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
			event := loadFixture(c)

			events, err := env.AWSEventToEvents(event)
			require.NoError(t, err)
			require.Len(t, events, 1)
			e, ok := events[0].(UpsertEvent)
			require.True(t, ok)
			require.Equal(t, e.ServiceName, s1.ServiceName)
			require.Equal(t, e.ARN, s1.ARN)
		})
	}

	t.Run("with an unsupported event name", func(t *testing.T) {
		event := loadFixture("unsupported")
		_, err := env.AWSEventToEvents(event)
		require.Error(t, err)
	})
}

func TestGetLambdaData(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:111111111111:function:lambda-1234"
	makeService := func(enterprise bool, alias string) UpsertEvent {
		e := UpsertEvent{
			ARN:                arn,
			PayloadPassthrough: true,
			InvocationMode:     asynchronousInvocationMode,
			ServiceName:        "lambda-1234",
		}
		if enterprise {
			e.EnterpriseMeta = &EnterpriseMeta{Namespace: "n", Partition: "p"}
		}
		if alias != "" {
			e.ServiceName = e.ServiceName + "-" + alias
			e.ARN = e.ARN + ":" + alias
		}
		return e
	}
	disabledService := makeService(false, "")
	serviceWithInvalidInvocationMode := makeService(false, "")
	serviceWithInvalidInvocationMode.InvocationMode = "invalid"

	cases := map[string]struct {
		arn          string
		upsertEvents []UpsertEventPlusMeta
		expected     []Event
		err          bool
		enterprise   bool
		partitions   []string
	}{
		"Invalid arn": {
			arn: "1234",
			err: true,
		},
		"Error fetching tags": {
			arn:          arn,
			err:          true,
			upsertEvents: []UpsertEventPlusMeta{},
		},
		"Enterprise meta is passed without enterprise Consul": {
			arn: arn,
			err: true,
			upsertEvents: []UpsertEventPlusMeta{
				{
					UpsertEvent: UpsertEvent{
						ServiceName:    "lambda-1234",
						EnterpriseMeta: &EnterpriseMeta{Namespace: "n", Partition: "p"},
					},
				},
			},
		},
		"Everything is passed - Enterprise": {
			arn: arn,
			err: false,
			upsertEvents: []UpsertEventPlusMeta{
				{
					UpsertEvent:   makeService(true, ""),
					CreateService: true,
				},
			},
			enterprise: true,
			expected: []Event{
				makeService(true, ""),
			},
			partitions: []string{"p"},
		},
		"Ignoring unhandled partitions - Enterprise": {
			arn: arn,
			err: false,
			upsertEvents: []UpsertEventPlusMeta{
				{
					UpsertEvent: makeService(true, ""),
				},
			},
			enterprise: true,
			partitions: []string{},
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
			expected:   []Event{DeleteEvent{EnterpriseMeta: nil, ServiceName: "lambda-1234"}},
		},
		"Everything is passed - OSS": {
			arn: arn,
			err: false,
			upsertEvents: []UpsertEventPlusMeta{
				{
					UpsertEvent:   makeService(false, ""),
					CreateService: true,
				},
			},
			enterprise: false,
			expected: []Event{
				makeService(false, ""),
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
					UpsertEvent:   makeService(false, ""),
					Aliases:       []string{"a1", "a2"},
					CreateService: true,
				},
			},
			enterprise: false,
			expected: []Event{
				makeService(false, ""),
				makeService(false, "a1"),
				makeService(false, "a2"),
			},
		},
	}

	for n, c := range cases {
		t.Run(n, func(t *testing.T) {
			lambda := mockLambdaClient(c.upsertEvents...)
			env := mockEnvironment(lambda, nil)
			env.IsEnterprise = c.enterprise
			for _, p := range c.partitions {
				env.Partitions[p] = struct{}{}
			}

			events, err := env.GetLambdaData(arn)
			if c.err {
				require.Error(t, err)
				return
			}

			require.Equal(t, c.expected, events)
		})
	}
}

func TestFullSyncData(t *testing.T) {
	enterprise := enterpriseFlag()

	var enterpriseMeta *EnterpriseMeta
	if enterprise {
		enterpriseMeta = &EnterpriseMeta{
			Namespace: "ns1",
			Partition: "ap1",
		}
	}

	s1 := UpsertEvent{
		ServiceName:    "lambda-1234",
		ARN:            "arn:aws:lambda:us-east-1:111111111111:function:lambda-1234",
		EnterpriseMeta: enterpriseMeta,
		InvocationMode: "SYNCHRONOUS",
	}
	service1 := UpsertEventPlusMeta{
		UpsertEvent:   s1,
		CreateService: true,
	}
	disabledService1 := service1
	disabledService1.CreateService = false

	s1prod := UpsertEvent{
		ServiceName:    "lambda-1234-prod",
		ARN:            s1.ARN + ":prod",
		EnterpriseMeta: enterpriseMeta,
		InvocationMode: "SYNCHRONOUS",
	}
	s1dev := UpsertEvent{
		ServiceName:    "lambda-1234-dev",
		ARN:            s1.ARN + ":dev",
		EnterpriseMeta: enterpriseMeta,
		InvocationMode: "SYNCHRONOUS",
	}
	service1WithAlias := UpsertEventPlusMeta{
		UpsertEvent:   s1,
		Aliases:       []string{"prod", "dev"},
		CreateService: true,
	}

	type caseData struct {
		// Set up consul state
		SeedConsulState []UpsertEventPlusMeta
		// Set up Lambda state
		SeedLambdaState []UpsertEventPlusMeta
		ExpectedEvents  []Event
		Partitions      []string
	}

	cases := map[string]*caseData{
		"Add a service": {
			SeedLambdaState: []UpsertEventPlusMeta{service1},
			ExpectedEvents:  []Event{s1},
		},
		"Remove a service": {
			SeedConsulState: []UpsertEventPlusMeta{service1},
			ExpectedEvents: []Event{
				DeleteEvent{
					ServiceName:    "lambda-1234",
					EnterpriseMeta: enterpriseMeta,
				}},
		},
		"Ignore Lambdas without create service meta": {
			SeedLambdaState: []UpsertEventPlusMeta{disabledService1},
			ExpectedEvents:  []Event{},
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
			env.IsEnterprise = enterprise
			for _, p := range c.Partitions {
				env.Partitions[p] = struct{}{}
			}

			for _, e := range c.SeedConsulState {
				err := e.Reconcile(env)
				require.NoError(t, err)
			}

			events, err := env.FullSyncData()
			require.NoError(t, err)
			require.ElementsMatch(t, c.ExpectedEvents, events)
		})
	}
}
