package main

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/stretchr/testify/require"
)

func TestUpsertAndDelete(t *testing.T) {
	enterprise := enterpriseFlag()
	server, err := testutil.NewTestServerConfigT(t, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = server.Stop()
	})

	consulClient, err := api.NewClient(&api.Config{Address: server.HTTPAddr})
	require.NoError(t, err)

	serviceName := "service"

	env := mockEnvironment(mockLambdaClient(), consulClient)
	type caseData struct {
		EnterpriseMeta *EnterpriseMeta
	}
	cases := make(map[string]caseData)

	if enterprise {
		cases["default partition and namespace"] = caseData{
			EnterpriseMeta: &EnterpriseMeta{
				Namespace: "default",
				Partition: "default",
			},
		}
		cases["partitions and namespaces"] = caseData{
			EnterpriseMeta: &EnterpriseMeta{
				Namespace: "ns1",
				Partition: "ap1",
			},
		}

		_, _, err = consulClient.Partitions().Create(context.Background(), &api.Partition{Name: "ap1"}, nil)
		require.NoError(t, err)
		_, _, err = consulClient.Namespaces().Create(&api.Namespace{Name: "ns1", Partition: "ap1"}, &api.WriteOptions{
			Partition: "ap1",
		})
		require.NoError(t, err)
	} else {
		cases["OSS"] = caseData{}
	}

	for n, c := range cases {
		upsertEvent := UpsertEvent{
			PayloadPassthrough: true,
			ServiceName:        serviceName,
			ARN:                "arn",
			EnterpriseMeta:     c.EnterpriseMeta,
		}
		deleteEvent := DeleteEvent{ServiceName: serviceName, EnterpriseMeta: c.EnterpriseMeta}

		t.Run(n, func(t *testing.T) {
			t.Run("Creating the service", func(t *testing.T) {
				err := upsertEvent.Reconcile(env)
				require.NoError(t, err)

				assertConsulState(t, consulClient, env, upsertEvent, 1)
			})

			t.Run("Enabling the service with meta", func(t *testing.T) {
				err := upsertEvent.Reconcile(env)
				require.NoError(t, err)

				assertConsulState(t, consulClient, env, upsertEvent, 1)
			})

			t.Run("Deleting the service", func(t *testing.T) {
				err := deleteEvent.Reconcile(env)
				require.NoError(t, err)

				assertConsulState(t, consulClient, env, upsertEvent, 0)
			})
		})
	}
}

func assertConsulState(t *testing.T, consulClient *api.Client, env Environment, event UpsertEvent, count int) {
	var queryOptions *api.QueryOptions
	if event.EnterpriseMeta != nil {
		queryOptions = &api.QueryOptions{
			Partition: event.EnterpriseMeta.Partition,
			Namespace: event.EnterpriseMeta.Namespace,
		}
	}
	services, _, err := consulClient.Catalog().Service(event.ServiceName, "", queryOptions)
	require.NoError(t, err)
	require.Len(t, services, count)

	entries, _, err := consulClient.ConfigEntries().List(api.ServiceDefaults, queryOptions)
	require.NoError(t, err)
	require.Len(t, entries, count)
	if count == 1 {
		require.Equal(t, event.ServiceName, entries[0].GetName())
	}
}

func enterpriseFlag() bool {
	re := regexp.MustCompile("^-+enterprise$")
	for _, a := range os.Args {
		if re.Match([]byte(strings.ToLower(a))) {
			return true
		}
	}
	return false
}
