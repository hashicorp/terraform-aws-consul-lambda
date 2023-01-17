package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-multierror"

	"github.com/hashicorp/terraform-aws-consul-lambda/consul-lambda/structs"
)

const (
	prefix = "serverless.consul.hashicorp.com/v1alpha1/lambda"

	// The supported lambda tags used to register the lambda function

	// enabledTag Enables the Lambda registrator to sync the Lambda with Consul.
	enabledTag = prefix + "/enabled"
	// payloadPassthroughTag determines if the body Envoy receives is converted to JSON or directly passed to Lambda.
	payloadPassthroughTag = prefix + "/payload-passthrough"
	// datacenterTag specifies the Consul datacenter in which to register the service.
	datacenterTag = prefix + "/datacenter"
	// partitionTag specifies the Consul partition the service is registered in.
	partitionTag = prefix + "/partition"
	// namespaceTag specifies the Consul namespace the service is registered in.
	namespaceTag = prefix + "/namespace"
	// aliasesTag Specifies a +-separated string of Lambda aliases that are registered into Consul.
	// For example, if set to dev+staging+prod, the dev, staging, and prod aliases of the Lambda function are
	// registered into Consul.
	aliasesTag = prefix + "/aliases"
	// invocationModeTag Specifies the Lambda invocation mode Consul uses to invoke the Lambda.
	invocationModeTag = prefix + "/invocation-mode"
)

const (
	asynchronousInvocationMode = "ASYNCHRONOUS"
	synchronousInvocationMode  = "SYNCHRONOUS"
)

var (
	errARNUndefined  = errors.New("arn isn't populated")
	errNotEnterprise = errors.New("namespaces and admin partitions require Consul enterprise")
)

type AWSEvent struct {
	Detail Detail `json:"detail"`
}

type Detail struct {
	EventID           string            `json:"eventID"`
	ErrorCode         string            `json:"errorCode"`
	EventName         string            `json:"eventName"`
	ResponseElements  ResponseElements  `json:"responseElements"`
	RequestParameters RequestParameters `json:"requestParameters"`
}

type ResponseElements struct {
	FunctionArn string `json:"functionArn"`
}

type RequestParameters struct {
	FunctionName string `json:"functionName"`
	Resource     string `json:"resource"`
}

// AWSEventToEvents converts an AWSEvent to a list of Events required to sync Lambda state with Consul.
func (env Environment) AWSEventToEvents(ctx context.Context, event AWSEvent) ([]Event, error) {
	var events []Event
	var arn string
	switch event.Detail.EventName {
	case "CreateFunction20150331", "CreateFunction":
		arn = event.Detail.ResponseElements.FunctionArn
	case "TagResource20170331v2", "TagResource20170331", "TagResource",
		"UntagResource20170331v2", "UntagResource20170331", "UntagResource":
		arn = event.Detail.RequestParameters.Resource
	default:
		return events, fmt.Errorf("unsupported event kind %s", event.Detail.EventName)
	}

	if arn == "" {
		return events, errARNUndefined
	}

	fn, err := env.Lambda.GetFunction(ctx, arn)
	if err != nil {
		return events, err
	}

	lambdaEvents, err := env.GetLambdaEvents(fn)
	if err != nil {
		return events, err
	}

	events = append(events, lambdaEvents...)

	return events, nil
}

const (
	// `,` isn't allowed
	// https://docs.aws.amazon.com/directoryservice/latest/devguide/API_Tag.html
	listSeparator = "+"
)

// GetLambdaEvents inspects the current state of the given Lambda function and returns the list of
// Events that are required to reconcile the function's state with Consul.
func (env Environment) GetLambdaEvents(fn LambdaFunction) ([]Event, error) {
	datacenter := ""
	createService := false
	payloadPassthrough := false
	invocationMode := synchronousInvocationMode
	var aliases []string

	tags := fn.Tags

	// The service name defaults to the name of the Lambda function
	serviceName := fn.Name

	if v, ok := tags[datacenterTag]; ok {
		datacenter = v
	}

	// If configured to manage a specific datacenter, ignore events from Lambdas in other datacenters.
	if env.Datacenter != "" && env.Datacenter != datacenter {
		env.Logger.Debug("ignoring function from remote dc", "service", serviceName, "service dc", datacenter, "dc", env.Datacenter)
		return nil, nil
	}

	if v, ok := tags[enabledTag]; ok {
		createService = v == "true"
	}

	if v, ok := tags[payloadPassthroughTag]; ok {
		payloadPassthrough = v == "true"
	}

	if v, ok := tags[invocationModeTag]; ok {
		invocationMode = v
		switch invocationMode {
		case asynchronousInvocationMode, synchronousInvocationMode:
		default:
			return nil, fmt.Errorf("invalid invocation mode: %s", invocationMode)
		}
	}

	// Get enterprise metadata from the tags. This will be nil for OSS.
	em := structs.NewEnterpriseMeta(tags[partitionTag], tags[namespaceTag])
	if !env.IsEnterprise && em != nil {
		return nil, errNotEnterprise
	}

	if env.IsEnterprise && em == nil {
		// If enterprise but no tags were provided, then use the default AP and NS.
		em = structs.NewEnterpriseMeta("default", "default")
	}

	// Ignore events in unhandled partitions.
	if env.IsEnterprise && em != nil {
		if _, ok := env.Partitions[em.Partition]; !ok {
			return nil, nil
		}
	}

	if aliasesRaw, ok := tags[aliasesTag]; ok {
		aliases = strings.Split(aliasesRaw, listSeparator)
	}

	var events []Event

	if createService {
		baseUpsertEvent := UpsertEvent{
			Service: structs.Service{
				Name:           serviceName,
				Datacenter:     datacenter,
				EnterpriseMeta: em,
			},
			LambdaArguments: LambdaArguments{
				ARN:                fn.ARN,
				PayloadPassthrough: payloadPassthrough,
				InvocationMode:     invocationMode,
			},
		}

		events = append(events, baseUpsertEvent)

		for _, aliasName := range aliases {
			e := baseUpsertEvent.AddAlias(aliasName)
			events = append(events, e)
		}
	} else {
		baseDeleteEvent := DeleteEvent{structs.Service{
			Name:           serviceName,
			Datacenter:     datacenter,
			EnterpriseMeta: em,
		}}

		events = append(events, baseDeleteEvent)

		for _, aliasName := range aliases {
			e := baseDeleteEvent.AddAlias(aliasName)
			events = append(events, e)
		}
	}

	return events, nil
}

func (env Environment) FullSyncData(ctx context.Context) ([]Event, error) {
	lambdas, err := env.getLambdas(ctx)
	if err != nil {
		return nil, err
	}
	env.Logger.Debug("retrieved lambdas", "lambdas", lambdas)

	enterpriseMetas, err := env.getEnterpriseMetas()
	if err != nil {
		return nil, err
	}
	env.Logger.Debug("retrieved enterpriseMetas", "enterpriseMetas", enterpriseMetas)

	// EnterpriseMeta is nil for OSS Consul.
	consulServices, err := env.getConsulServices(enterpriseMetas)
	if err != nil {
		return nil, err
	}
	env.Logger.Debug("retrieved consulServices", "consulServices", consulServices)

	events := env.constructUpsertEvents(lambdas, consulServices)
	return append(events, env.constructDeleteEvents(lambdas, consulServices)...), nil
}

type eventMap map[structs.EnterpriseMeta]map[string]Event
type serviceMap map[structs.EnterpriseMeta]map[string]struct{}

// getLambdas makes requests to the AWS APIs to get data about every Lambda and
// constructs events to register or deregister those Lambdas with Consul.
func (env Environment) getLambdas(ctx context.Context) (eventMap, error) {
	var resultErr error
	lambdas := make(eventMap)

	funcs, err := env.Lambda.ListFunctions(ctx)
	if err != nil {
		return lambdas, err
	}

	// TODO: could do this processing concurrently
	for _, fn := range funcs {
		events, err := env.GetLambdaEvents(fn)
		if err != nil {
			resultErr = multierror.Append(resultErr, err)
			continue
		}

		for _, event := range events {
			switch e := event.(type) {
			case UpsertEvent:
				var em structs.EnterpriseMeta
				if e.EnterpriseMeta != nil {
					em = *e.EnterpriseMeta
				}
				if lambdas[em] == nil {
					lambdas[em] = make(map[string]Event)
				}
				lambdas[em][e.Name] = event

			case DeleteEvent:
				var em structs.EnterpriseMeta
				if e.EnterpriseMeta != nil {
					em = *e.EnterpriseMeta
				}
				if lambdas[em] == nil {
					lambdas[em] = make(map[string]Event)
				}
				lambdas[em][e.Name] = event
			}
		}
	}

	return lambdas, resultErr
}

// getEnterpriseMetas determines which Consul partitions will be synced.
// A slice with one nil entry is used to indicate OSS Consul.
func (env Environment) getEnterpriseMetas() ([]structs.EnterpriseMeta, error) {
	var enterpriseMetas []structs.EnterpriseMeta
	if env.IsEnterprise {
		for partition := range env.Partitions {
			namespaces, _, err := env.ConsulClient.Namespaces().List(&api.QueryOptions{Partition: partition})
			if err != nil {
				return nil, err
			}

			for _, namespace := range namespaces {
				enterpriseMetas = append(enterpriseMetas, structs.EnterpriseMeta{
					Partition: partition,
					Namespace: namespace.Name,
				})
			}
		}
	} else {
		enterpriseMetas = append(enterpriseMetas, structs.EnterpriseMeta{})
	}

	return enterpriseMetas, nil
}

// getConsulServices retrieves all Consul services that are managed by Lambda registrator.
func (env Environment) getConsulServices(enterpriseMetas []structs.EnterpriseMeta) (serviceMap, error) {
	consulServices := make(serviceMap)
	for _, em := range enterpriseMetas {
		var queryOptions *api.QueryOptions
		if em.Partition != "" && em.Namespace != "" {
			queryOptions = &api.QueryOptions{
				Partition: em.Partition,
				Namespace: em.Namespace,
			}
		}
		env.Logger.Debug("querying consul catalog")
		services, _, err := env.ConsulClient.Catalog().Services(queryOptions)
		if err != nil {
			return nil, err
		}
		env.Logger.Debug("got service catalog", "services", services)
		consulServices[em] = make(map[string]struct{})
		for serviceName, tags := range services {
			for _, t := range tags {
				if managedLambdaTag == t {
					consulServices[em][serviceName] = struct{}{}
					break
				}
			}
		}
	}

	return consulServices, nil
}

// constructUpsertEvents determines which upsert events need to be processed to
// synchronize Consul with Lambda.
func (env Environment) constructUpsertEvents(lambdas eventMap, consulServices serviceMap) []Event {
	var events []Event

	for enterpriseMeta, lambdaEvents := range lambdas {
		for serviceName, event := range lambdaEvents {
			switch e := event.(type) {
			case UpsertEvent:
				if consulEvents, ok := consulServices[enterpriseMeta]; ok {
					if _, ok := consulEvents[serviceName]; !ok {
						events = append(events, e)
					}
				} else {
					events = append(events, e)
				}
			case DeleteEvent:
				if consulEvents, ok := consulServices[enterpriseMeta]; ok {
					if _, ok := consulEvents[serviceName]; ok {
						events = append(events, e)
					}
				}
			}
		}
	}

	return events
}

// constructUpsertEvents determines which delete events need to be processed to
// synchronize Consul with Lambda.
func (env Environment) constructDeleteEvents(lambdas eventMap, consulServices serviceMap) []Event {
	var events []Event
	// Constructing delete events for services that need to be deregistered in Consul
	for enterpriseMeta, consulService := range consulServices {
		for serviceName := range consulService {
			deleteEvent := DeleteEvent{structs.Service{
				Name:           serviceName,
				EnterpriseMeta: structs.NewEnterpriseMeta(enterpriseMeta.Partition, enterpriseMeta.Namespace)}}
			if lambdaEvents, ok := lambdas[enterpriseMeta]; ok {
				if _, ok := lambdaEvents[serviceName]; !ok {
					events = append(events, deleteEvent)
				}
			} else {
				events = append(events, deleteEvent)
			}
		}
	}

	return events
}
