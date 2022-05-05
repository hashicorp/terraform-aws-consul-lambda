package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAWSEventToEvents(t *testing.T) {
	arn := "arn:aws:lambda:us-east-1:111111111111:function:lambda-1234"
	s1 := UpsertEvent{
		CreateService: true,
		ServiceName:   "lambda-1234",
		ARN:           arn,
	}
	s1WithAliases := UpsertEventPlusAliases{
		UpsertEvent: s1,
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
			require.Equal(t, e.CreateService, s1.CreateService)
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
			CreateService:      true,
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
	cases := map[string]struct {
		arn          string
		upsertEvents []UpsertEventPlusAliases
		expected     []UpsertEvent
		err          bool
		enterprise   bool
	}{
		"Invalid arn": {
			arn: "1234",
			err: true,
		},
		"Error fetching tags": {
			arn:          arn,
			err:          true,
			upsertEvents: []UpsertEventPlusAliases{},
		},
		"Enterprise meta is passed without enterprise Consul": {
			arn: arn,
			err: true,
			upsertEvents: []UpsertEventPlusAliases{
				{
					UpsertEvent: UpsertEvent{
						CreateService:  true,
						ServiceName:    "lambda-1234",
						EnterpriseMeta: &EnterpriseMeta{Namespace: "n", Partition: "p"},
					},
				},
			},
		},
		"Everything is passed - Enterprise": {
			arn: arn,
			err: false,
			upsertEvents: []UpsertEventPlusAliases{
				{
					UpsertEvent: makeService(true, ""),
				},
			},
			enterprise: true,
			expected: []UpsertEvent{
				makeService(true, ""),
			},
		},
		"Everything is passed - OSS": {
			arn: arn,
			err: false,
			upsertEvents: []UpsertEventPlusAliases{
				{
					UpsertEvent: makeService(false, ""),
				},
			},
			enterprise: false,
			expected: []UpsertEvent{
				makeService(false, ""),
			},
		},
		"Aliases": {
			arn: arn,
			err: false,
			upsertEvents: []UpsertEventPlusAliases{
				{
					UpsertEvent: makeService(false, ""),
					Aliases:     []string{"a1", "a2"},
				},
			},
			enterprise: false,
			expected: []UpsertEvent{
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

			events, err := env.GetLambdaData(arn)
			if c.err {
				require.Error(t, err)
				return
			}

			require.Equal(t, c.expected, events)
		})
	}
}
