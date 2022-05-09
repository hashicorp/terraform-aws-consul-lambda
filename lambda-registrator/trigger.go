package main

import (
	"errors"
	"fmt"
	"strings"

	sdkARN "github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/lambda"
)

const (
	prefix                = "serverless.consul.hashicorp.com/v1alpha1/lambda"
	enabledTag            = prefix + "/enabled"
	arnTag                = prefix + "/arn"
	payloadPassthroughTag = prefix + "/payload-passhthrough"
	regionTag             = prefix + "/region"
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
	regionUndefinedErr = errors.New("region isn't populated")
	arnUndefinedErr    = errors.New("arn isn't populated")
	notEnterpriseErr   = errors.New("namespaces and admin partitions can't be used with open source Consul")
)

type AWSEvent struct {
	Detail Detail `json:"detail"`
}

type Detail struct {
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

type EnterpriseMeta struct {
	Namespace string
	Partition string
}

func (e Environment) AWSEventToEvents(event AWSEvent) ([]Event, error) {
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
		return events, arnUndefinedErr
	}

	upsertEvents, err := e.GetLambdaData(arn)

	if err != nil {
		return events, err
	}

	for _, e := range upsertEvents {
		events = append(events, e)
	}

	return events, nil
}

func (e Environment) GetLambdaData(arn string) ([]Event, error) {
	createService := false
	payloadPassthrough := false
	invocationMode := synchronousInvocationMode
	var aliases []string

	// This is terrible, but it saves tons of API calls to GetFunction just for
	// the function name.
	parsedARN, err := sdkARN.Parse(arn)
	if err != nil {
		return nil, err
	}
	functionName := ""
	if i := strings.IndexByte(parsedARN.Resource, ':'); i != -1 {
		functionName = parsedARN.Resource[i+1:]
	}

	tagOutput, err := e.Lambda.ListTags(&lambda.ListTagsInput{
		Resource: &arn,
	})

	if err != nil {
		return nil, err
	}

	tags := tagOutput.Tags

	if tags[enabledTag] != nil {
		createService = *tags[enabledTag] == "true"
	}

	if tags[payloadPassthroughTag] != nil {
		payloadPassthrough = *tags[payloadPassthroughTag] == "true"
	}

	if tags[invocationModeTag] != nil {
		invocationMode = *tags[invocationModeTag]
		switch invocationMode {
		case asynchronousInvocationMode, synchronousInvocationMode:
		default:
			return nil, fmt.Errorf("invalid invocation mode: %s", invocationMode)
		}
	}

	var em *EnterpriseMeta
	if tags[namespaceTag] != nil {
		em = &EnterpriseMeta{Namespace: *tags[namespaceTag], Partition: "default"}
	}

	if tags[partitionTag] != nil {
		partition := *tags[partitionTag]
		if em == nil {
			em = &EnterpriseMeta{Namespace: "default", Partition: partition}
		} else {
			em.Partition = partition
		}
	}

	if !e.IsEnterprise && em != nil {
		return nil, notEnterpriseErr
	}

	if aliasesRaw, ok := tags[aliasesTag]; ok {
		aliases = strings.Split(*aliasesRaw, ",")
	}

	var events []Event

	if createService {
		baseUpsertEvent := UpsertEvent{
			PayloadPassthrough: payloadPassthrough,
			ServiceName:        functionName,
			ARN:                arn,
			EnterpriseMeta:     em,
			InvocationMode:     invocationMode,
		}

		events = append(events, baseUpsertEvent)

		for _, aliasName := range aliases {
			e := baseUpsertEvent.AddAlias(aliasName)
			events = append(events, e)
		}
	} else {
		baseDeleteEvent := DeleteEvent{
			ServiceName:    functionName,
			EnterpriseMeta: em,
		}

		events = append(events, baseDeleteEvent)

		for _, aliasName := range aliases {
			e := baseDeleteEvent.AddAlias(aliasName)
			events = append(events, e)
		}
	}

	return events, nil
}

func (e Environment) FullSyncData() ([]Event, error) {
	return nil, nil
}
