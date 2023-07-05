// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/structs"
)

const (
	managedLambdaTag = "managed-by-lambda-registrator"

	arnField                = "arn"
	invocationModeField     = "invocationMode"
	payloadPassthroughField = "payloadPassthrough"
)

// UpsertEvent struct holds data for an event that triggers the upserting of a Lambda function.
type UpsertEvent struct {
	structs.Service
	LambdaArguments
}

// LambdaArguments configuration for an extension that patches Envoy resources for lambda
type LambdaArguments struct {
	// PayloadPassthrough determines if the body Envoy receives is converted to JSON or directly passed to Lambda.
	PayloadPassthrough bool
	// ARN specifies the AWS ARN for the service's Lambda. ARN must be set to a valid Lambda function ARN.
	ARN string
	// 	InvocationMode Determines if Consul configures the Lambda to be invoked using the `synchronous`
	//	or `asynchronous` invocation mode
	InvocationMode string
}

func (e LambdaArguments) toConsulServiceConfigEntry(name string) *api.ServiceConfigEntry {
	serviceDefaults := &api.ServiceConfigEntry{
		Kind:     api.ServiceDefaults,
		Name:     name,
		Protocol: "http",
		EnvoyExtensions: []api.EnvoyExtension{
			{
				Name:     api.BuiltinAWSLambdaExtension,
				Required: false,
				Arguments: map[string]interface{}{
					arnField:                e.ARN,
					invocationModeField:     e.InvocationMode,
					payloadPassthroughField: e.PayloadPassthrough,
				},
			},
		},
	}

	return serviceDefaults
}

// Identifier returns the ARN of the Lambda function being upserted.
func (e UpsertEvent) Identifier() string {
	return e.ARN
}

// Reconcile reconciles lambda with the state of Consul and performs the necessary steps to upsert a Lambda.
func (e UpsertEvent) Reconcile(env Environment) error {
	env.Logger.Info("Upserting Lambda", "arn", e.ARN)
	env.Logger.Debug("Storing service defaults config entry", "arn", e.ARN)
	err := env.storeServiceDefaults(e)
	if err != nil {
		return err
	}

	env.Logger.Debug("Registering service", "arn", e.ARN)
	err = env.registerService(e)
	if err != nil {
		return err
	}

	return env.upsertTLSData(e)
}

// AddAlias sets the UpserEvent Name and ARN by appending on the alias in the form `-alias`
func (e UpsertEvent) AddAlias(alias string) UpsertEvent {
	e.Name = fmt.Sprintf("%s-%s", e.Name, alias)
	e.ARN = fmt.Sprintf("%s:%s", e.ARN, alias)
	return e
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
			ID:      e.Name,
			Service: e.Name,
			Tags:    []string{managedLambdaTag},
		},
	}

	_, err := env.ConsulClient.Catalog().Register(registration, WriteOptions(e.Service))
	return err
}

func (env Environment) storeServiceDefaults(e UpsertEvent) error {
	serviceDefaults := e.toConsulServiceConfigEntry(e.Name)

	// There is no need for CAS because we are completely regenerating the service
	// defaults config entry.
	_, _, err := env.ConsulClient.ConfigEntries().Set(serviceDefaults, WriteOptions(e.Service))

	return err
}

func (env Environment) upsertTLSData(e UpsertEvent) error {
	var advancedTier bool
	if !env.IsManagingTLS() {
		return nil
	}

	env.Logger.Debug("upserting mTLS data", "service", e.Name)

	// Retrieve Consul root CA
	// TODO: Should optimize this flow so that we only request the root CA once, not for every service.
	// TODO: Retrieve the root CA IFF it is expiring "soon".
	caRootList, _, err := env.ConsulClient.Agent().ConnectCARoots(nil)
	if err != nil {
		return fmt.Errorf("failed to retrieve Consul root CA: %w", err)
	}

	// Use the first active root CA cert
	var caRoot *api.CARoot
	for _, root := range caRootList.Roots {
		if root.ID == caRootList.ActiveRootID {
			caRoot = root
			break
		}
	}
	if caRoot == nil {
		return fmt.Errorf("failed to find an active CA root cert: %w", err)
	}

	// Retrieve the leaf for this service
	leafCert, _, err := env.ConsulClient.Agent().ConnectCALeaf(e.Name, QueryOptions(e.Service))
	if err != nil {
		return fmt.Errorf("failed to retrieve leaf cert for %s: %w", e.Name, err)
	}

	extData, err := json.Marshal(structs.ExtensionData{
		PrivateKeyPEM: leafCert.PrivateKeyPEM,
		CertPEM:       leafCert.CertPEM,
		RootCertPEM:   caRoot.RootCertPEM,
		TrustDomain:   caRootList.TrustDomain,
		// TODO: cluster peering support
	})
	if err != nil {
		return fmt.Errorf("failed to marshal extension data: %w", err)
	}

	// Create the info for this service
	service := structs.Service{
		Name:        e.Name,
		Datacenter:  e.Datacenter,
		TrustDomain: caRootList.TrustDomain,
	}
	if e.EnterpriseMeta != nil {
		service.EnterpriseMeta = e.EnterpriseMeta
	}
	path := fmt.Sprintf("%s%s", env.ExtensionDataPrefix, service.ExtensionPath())

	advancedTier, err = strconv.ParseBool(os.Getenv("CONSUL_ADVANCED_PARAMS"))
	if err != nil {
		env.Logger.Debug("Unable to parse (true, false) setting to standard tier parameter")
	}
	// TODO: do we need to pass a context in here?.. like from the lambda entrypoint
	// so that this call can be canceled if necessary.
	return env.Store.Set(context.Background(), path, string(extData), advancedTier)
}

// QueryOptions takes in a structs.Service and returns a pointer to an api.QueryOptions struct.
// If the service has an EnterpriseMeta field, it sets the Partition and Namespace fields in the QueryOptions struct.
func QueryOptions(s structs.Service) *api.QueryOptions {
	opts := &api.QueryOptions{Datacenter: s.Datacenter}
	if s.EnterpriseMeta != nil {
		opts.Partition = s.EnterpriseMeta.Partition
		opts.Namespace = s.EnterpriseMeta.Namespace
	}
	return opts
}

// WriteOptions takes in a structs.Service and returns a pointer to an api.WriteOptions struct.
// If the service has an EnterpriseMeta field, it sets the Partition and Namespace fields in the WriteOptions struct.
func WriteOptions(s structs.Service) *api.WriteOptions {
	opts := &api.WriteOptions{Datacenter: s.Datacenter}
	if s.EnterpriseMeta != nil {
		opts.Partition = s.EnterpriseMeta.Partition
		opts.Namespace = s.EnterpriseMeta.Namespace
	}
	return opts
}
