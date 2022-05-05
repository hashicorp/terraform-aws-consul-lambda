package main

import (
	"errors"
	"fmt"
	sdkARN "github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/lambda"
	"strings"
)

const (
	prefix                = "serverless.consul.hashicorp.com/v1alpha1"
	enabledTag            = prefix + "/lambda/enabled"
	arnTag                = prefix + "/lambda/arn"
	payloadPassthroughTag = prefix + "/lambda/payload-passhthrough"
	regionTag             = prefix + "/lambda/region"
	partitionTag          = prefix + "/lambda/partition"
	namespaceTag          = prefix + "/lambda/namespace"
	aliasesTag            = prefix + "/lambda/aliases"
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

func (e Environment) GetLambdaData(arn string) ([]UpsertEvent, error) {
	createService := false
	payloadPassthrough := false
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

	baseUpsertEvent := UpsertEvent{
		CreateService:      createService,
		PayloadPassthrough: payloadPassthrough,
		ServiceName:        functionName,
		ARN:                arn,
		EnterpriseMeta:     em,
	}

	upsertEvents := []UpsertEvent{baseUpsertEvent}

	for _, aliasName := range aliases {
		e := baseUpsertEvent
		e.ServiceName = fmt.Sprintf("%s-%s", baseUpsertEvent.ServiceName, aliasName)
		e.ARN = fmt.Sprintf("%s:%s", arn, aliasName)
		upsertEvents = append(upsertEvents, e)
	}

	return upsertEvents, nil
}
