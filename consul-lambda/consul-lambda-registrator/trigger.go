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
	prefix                = "serverless.consul.hashicorp.com/v1alpha1/lambda"
	enabledTag            = prefix + "/enabled"
	arnTag                = prefix + "/arn"
	payloadPassthroughTag = prefix + "/payload-passthrough"
	regionTag             = prefix + "/region"
	datacenterTag         = prefix + "/datacenter"
	partitionTag          = prefix + "/partition"
	namespaceTag          = prefix + "/namespace"
	aliasesTag            = prefix + "/aliases"
	invocationModeTag     = prefix + "/invocation-mode"
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
func (e Environment) AWSEventToEvents(ctx context.Context, event AWSEvent) ([]Event, error) {
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

	fn, err := e.Lambda.GetFunction(ctx, arn)
	if err != nil {
		return events, err
	}

	lambdaEvents, err := e.GetLambdaEvents(ctx, fn)
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
func (e Environment) GetLambdaEvents(ctx context.Context, fn LambdaFunction) ([]Event, error) {
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
	if e.Datacenter != "" && e.Datacenter != datacenter {
		e.Logger.Debug("ignoring function from remote dc", "service", serviceName, "service dc", datacenter, "dc", e.Datacenter)
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
	if !e.IsEnterprise && em != nil {
		return nil, errNotEnterprise
	}

	if e.IsEnterprise && em == nil {
		// If enterprise but no tags were provided, then use the default AP and NS.
		em = structs.NewEnterpriseMeta("default", "default")
	}

	// Ignore events in unhandled partitions.
	if e.IsEnterprise && em != nil {
		if _, ok := e.Partitions[em.Partition]; !ok {
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
			ARN:                fn.ARN,
			PayloadPassthrough: payloadPassthrough,
			InvocationMode:     invocationMode,
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

func (e Environment) FullSyncData(ctx context.Context) ([]Event, error) {
	lambdas, err := e.getLambdas(ctx)
	if err != nil {
		return nil, err
	}
	e.Logger.Debug("retrieved lambdas", "lambdas", lambdas)

	enterpriseMetas, err := e.getEnterpriseMetas()
	if err != nil {
		return nil, err
	}
	e.Logger.Debug("retrieved enterpriseMetas", "enterpriseMetas", enterpriseMetas)

	// EnterpriseMeta is nil for OSS Consul.
	consulServices, err := e.getConsulServices(enterpriseMetas)
	if err != nil {
		return nil, err
	}
	e.Logger.Debug("retrieved consulServices", "consulServices", consulServices)

	events := e.constructUpsertEvents(lambdas, consulServices)
	return append(events, e.constructDeleteEvents(lambdas, consulServices)...), nil
}

type eventMap map[*structs.EnterpriseMeta]map[string]Event
type serviceMap map[*structs.EnterpriseMeta]map[string]struct{}

// getLambdas makes requests to the AWS APIs to get data about every Lambda and
// constructs events to register or deregister those Lambdas with Consul.
func (e Environment) getLambdas(ctx context.Context) (eventMap, error) {
	var resultErr error
	lambdas := make(eventMap)

	funcs, err := e.Lambda.ListFunctions(ctx)
	if err != nil {
		return lambdas, err
	}

	// TODO: could do this processing concurrently
	for _, fn := range funcs {
		events, err := e.GetLambdaEvents(ctx, fn)
		if err != nil {
			resultErr = multierror.Append(resultErr, err)
			continue
		}

		for _, event := range events {
			switch e := event.(type) {
			case UpsertEvent:
				if lambdas[e.EnterpriseMeta] == nil {
					lambdas[e.EnterpriseMeta] = make(map[string]Event)
				}
				lambdas[e.EnterpriseMeta][e.Name] = event

			case DeleteEvent:
				if lambdas[e.EnterpriseMeta] == nil {
					lambdas[e.EnterpriseMeta] = make(map[string]Event)
				}
				lambdas[e.EnterpriseMeta][e.Name] = event
			}
		}
	}

	return lambdas, resultErr
}

// getEnterpriseMetas determines which Consul partitions will be synced.
// A slice with one nil entry is used to indicate OSS Consul.
func (e Environment) getEnterpriseMetas() ([]*structs.EnterpriseMeta, error) {
	var enterpriseMetas []*structs.EnterpriseMeta
	if e.IsEnterprise {
		for partition := range e.Partitions {
			namespaces, _, err := e.ConsulClient.Namespaces().List(&api.QueryOptions{Partition: partition})
			if err != nil {
				return nil, err
			}

			for _, namespace := range namespaces {
				enterpriseMetas = append(enterpriseMetas, &structs.EnterpriseMeta{
					Partition: partition,
					Namespace: namespace.Name,
				})
			}
		}
	} else {
		enterpriseMetas = append(enterpriseMetas, nil)
	}

	return enterpriseMetas, nil
}

// getConsulServices retrieves all Consul services that are managed by Lambda registrator.
func (e Environment) getConsulServices(enterpriseMetas []*structs.EnterpriseMeta) (serviceMap, error) {
	consulServices := make(serviceMap)
	for _, em := range enterpriseMetas {
		var queryOptions *api.QueryOptions
		if em != nil {
			queryOptions = &api.QueryOptions{
				Partition: em.Partition,
				Namespace: em.Namespace,
			}
		}
		e.Logger.Debug("querying consul catalog")
		services, _, err := e.ConsulClient.Catalog().Services(queryOptions)
		if err != nil {
			return nil, err
		}
		e.Logger.Debug("got service catalog", "services", services)
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
func (e Environment) constructUpsertEvents(lambdas eventMap, consulServices serviceMap) []Event {
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
func (e Environment) constructDeleteEvents(lambdas eventMap, consulServices serviceMap) []Event {
	var events []Event
	// Constructing delete events for services that need to be deregistered in Consul
	for enterpriseMeta, consulService := range consulServices {
		for serviceName := range consulService {
			deleteEvent := DeleteEvent{structs.Service{Name: serviceName, EnterpriseMeta: enterpriseMeta}}
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
