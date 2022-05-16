package main

import (
	"fmt"
	"strconv"

	"github.com/hashicorp/consul/api"
)

const managedLambdaTag = "managed-by-lambda-registrator"

type UpsertEvent struct {
	PayloadPassthrough bool
	ServiceName        string
	ARN                string
	InvocationMode     string
	EnterpriseMeta     *EnterpriseMeta
}

func (e UpsertEvent) Identifier() string {
	return e.ARN
}

func (e UpsertEvent) Reconcile(env Environment) error {
	env.Logger.Info("Upserting Lambda", "arn", e.ARN)
	env.Logger.Debug("Storing service defaults config entry", "arn", e.ARN)
	err := env.storeServiceDefaults(e)
	if err != nil {
		return err
	}

	env.Logger.Debug("Registering service", "arn", e.ARN)
	return env.registerService(e)
}

func (env Environment) registerService(e UpsertEvent) error {
	registration := &api.CatalogRegistration{
		Node:           env.NodeName,
		SkipNodeUpdate: true,
		NodeMeta: map[string]string{
			"external-node":  "true",
			"external-probe": "true",
		},
		Service: &api.AgentService{
			ID:      e.ServiceName,
			Service: e.ServiceName,
			Tags:    []string{managedLambdaTag},
		},
	}

	_, err := env.ConsulClient.Catalog().Register(registration, e.writeOptions())
	return err
}

func (env Environment) storeServiceDefaults(e UpsertEvent) error {
	serviceDefaults := &api.ServiceConfigEntry{
		Kind:     api.ServiceDefaults,
		Name:     e.ServiceName,
		Protocol: "http",
		Meta: map[string]string{
			enabledTag:            "true",
			arnTag:                e.ARN,
			regionTag:             env.Region,
			payloadPassthroughTag: strconv.FormatBool(e.PayloadPassthrough),
		},
	}

	// There is no need for CAS because we are completely regenerating the service
	// defaults config entry.
	_, _, err := env.ConsulClient.ConfigEntries().Set(serviceDefaults, e.writeOptions())

	return err
}

func (e UpsertEvent) writeOptions() *api.WriteOptions {
	var writeOptions *api.WriteOptions
	if e.EnterpriseMeta != nil {
		writeOptions = &api.WriteOptions{
			Partition: e.EnterpriseMeta.Partition,
			Namespace: e.EnterpriseMeta.Namespace,
		}
	}

	return writeOptions
}

func (e UpsertEvent) AddAlias(alias string) UpsertEvent {
	e.ServiceName = fmt.Sprintf("%s-%s", e.ServiceName, alias)
	e.ARN = fmt.Sprintf("%s:%s", e.ARN, alias)
	return e
}
