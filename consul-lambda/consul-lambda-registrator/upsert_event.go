package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/structs"
)

const managedLambdaTag = "managed-by-lambda-registrator"

type UpsertEvent struct {
	structs.Service
	PayloadPassthrough bool
	ARN                string
	InvocationMode     string
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
	err = env.registerService(e)
	if err != nil {
		return err
	}

	return env.upsertTLSData(e)
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
	serviceDefaults := &api.ServiceConfigEntry{
		Kind:     api.ServiceDefaults,
		Name:     e.Name,
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
	_, _, err := env.ConsulClient.ConfigEntries().Set(serviceDefaults, WriteOptions(e.Service))

	return err
}

func (env Environment) upsertTLSData(e UpsertEvent) error {
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

	// TODO: do we need to pass a context in here?.. like from the lambda entrypoint
	// so that this call can be canceled if necessary.
	return env.Store.Set(context.Background(), path, string(extData))
}

func (e UpsertEvent) AddAlias(alias string) UpsertEvent {
	e.Name = fmt.Sprintf("%s-%s", e.Name, alias)
	e.ARN = fmt.Sprintf("%s:%s", e.ARN, alias)
	return e
}

func QueryOptions(s structs.Service) *api.QueryOptions {
	opts := &api.QueryOptions{Datacenter: s.Datacenter}
	if s.EnterpriseMeta != nil {
		opts.Partition = s.EnterpriseMeta.Partition
		opts.Namespace = s.EnterpriseMeta.Namespace
	}
	return opts
}

func WriteOptions(s structs.Service) *api.WriteOptions {
	opts := &api.WriteOptions{Datacenter: s.Datacenter}
	if s.EnterpriseMeta != nil {
		opts.Partition = s.EnterpriseMeta.Partition
		opts.Namespace = s.EnterpriseMeta.Namespace
	}
	return opts
}
