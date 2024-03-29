// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/structs"
)

// DeleteEvent struct holds data for an event that triggers the deletion of a Lambda function.
type DeleteEvent struct {
	structs.Service
}

// Identifier returns the name of the lambda function being deleted.
func (e DeleteEvent) Identifier() string {
	return e.Name
}

// Reconcile reconciles lambda with the state of Consul and performs the necessary steps to delete a Lambda.
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

	// Create the info for this service
	service := structs.Service{
		Name:       e.Name,
		Datacenter: e.Datacenter,
	}
	if e.EnterpriseMeta != nil {
		service.EnterpriseMeta = e.EnterpriseMeta
	}

	// TODO: do we need to pass a context in here?.. like from the lambda entrypoint
	// so that this call can be canceled if necessary.
	return env.Store.Delete(context.Background(),
		fmt.Sprintf("%s%s", env.ExtensionDataPrefix, service.ExtensionPath()))
}

func (e DeleteEvent) writeOptions() *api.WriteOptions {
	writeOptions := &api.WriteOptions{Datacenter: e.Datacenter}
	if e.EnterpriseMeta != nil {
		writeOptions.Partition = e.EnterpriseMeta.Partition
		writeOptions.Namespace = e.EnterpriseMeta.Namespace
	}
	return writeOptions
}

// AddAlias returns a new DeleteEvent for a Lambda function with an alias so that it can be removed from Consul
// when Reconcile is called.
func (e DeleteEvent) AddAlias(alias string) DeleteEvent {
	e.Name = fmt.Sprintf("%s-%s", e.Name, alias)
	return e
}
