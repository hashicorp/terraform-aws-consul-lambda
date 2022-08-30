package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/terraform-aws-consul-lambda-registrator/consul-lambda-registrator/structs"
)

type DeleteEvent struct {
	structs.Service
}

func (e DeleteEvent) Identifier() string {
	return e.Name
}

func (e DeleteEvent) Reconcile(env Environment) error {
	env.Logger.Info("Deleting Lambda service from Consul", "service-name", e.Name)

	env.Logger.Debug("Deleting service defaults config entry", "service-name", e.Name)
	err := env.deleteServiceDefaults(e)
	if err != nil {
		return err
	}

	env.Logger.Debug("De-registering service", "service-name", e.Name)
	err = env.deregisterService(e)
	if err != nil {
		return err
	}

	return env.deleteTLSData(e)
}

func (env Environment) deregisterService(event DeleteEvent) error {
	deregistration := &api.CatalogDeregistration{
		Node:      env.NodeName,
		ServiceID: event.Name,
	}
	_, err := env.ConsulClient.Catalog().Deregister(deregistration, event.writeOptions())
	return err
}

func (env Environment) deleteServiceDefaults(event DeleteEvent) error {
	_, err := env.ConsulClient.ConfigEntries().Delete(api.ServiceDefaults, event.Name, event.writeOptions())
	return err
}

func (env Environment) deleteTLSData(e DeleteEvent) error {
	if !env.IsManagingTLS() {
		return nil
	}

	env.Logger.Debug("deleting mTLS data", "service", e.Name)

	// TODO: do we need to pass a context in here?.. like from the lambda entrypoint
	// so that this call can be canceled if necessary.
	return env.Store.Delete(context.Background(), fmt.Sprintf("%s%s", env.ExtensionDataPath, e.ExtensionPath()))
}

func (e DeleteEvent) writeOptions() *api.WriteOptions {
	writeOptions := &api.WriteOptions{Datacenter: e.Datacenter}
	if e.EnterpriseMeta != nil {
		writeOptions.Partition = e.EnterpriseMeta.Partition
		writeOptions.Namespace = e.EnterpriseMeta.Namespace
	}
	return writeOptions
}

func (e DeleteEvent) AddAlias(alias string) DeleteEvent {
	e.Name = fmt.Sprintf("%s-%s", e.Name, alias)
	return e
}
