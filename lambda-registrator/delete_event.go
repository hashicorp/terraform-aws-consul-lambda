package main

import (
	"github.com/hashicorp/consul/api"
)

type DeleteEvent struct {
	ServiceName    string
	EnterpriseMeta *EnterpriseMeta
}

func (e DeleteEvent) Identifier() string {
	return e.ServiceName
}

func (e DeleteEvent) Reconcile(env Environment) error {
	env.Logger.Info("Deleting Lambda service from Consul", "service-name", e.ServiceName)

	env.Logger.Debug("Deleting service defaults config entry", "service-name", e.ServiceName)
	err := env.deleteServiceDefaults(e)
	if err != nil {
		return err
	}

	env.Logger.Debug("Deregistering service", "service-name", e.ServiceName)
	return env.deregisterService(e)
}

func (env Environment) deregisterService(event DeleteEvent) error {
	deregistration := &api.CatalogDeregistration{
		Node:      env.NodeName,
		ServiceID: event.ServiceName,
	}
	_, err := env.ConsulClient.Catalog().Deregister(deregistration, event.writeOptions())
	return err
}

func (env Environment) deleteServiceDefaults(event DeleteEvent) error {
	_, err := env.ConsulClient.ConfigEntries().Delete(api.ServiceDefaults, event.ServiceName, event.writeOptions())
	return err
}

func (e DeleteEvent) writeOptions() *api.WriteOptions {
	var writeOptions *api.WriteOptions
	if e.EnterpriseMeta != nil {
		writeOptions = &api.WriteOptions{
			Partition: e.EnterpriseMeta.Partition,
			Namespace: e.EnterpriseMeta.Namespace,
		}
	}

	return writeOptions
}
