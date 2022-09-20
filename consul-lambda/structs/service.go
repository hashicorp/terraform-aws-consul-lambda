package structs

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	internal        = "internal"
	version         = "v1"
	internalVersion = internal + "-" + version
)

type EnterpriseMeta struct {
	Namespace string
	Partition string
}

func NewEnterpriseMeta(ap, ns string) *EnterpriseMeta {
	if ap == "" && ns == "" {
		return nil
	}
	if ap == "" {
		ap = "default"
	}
	if ns == "" {
		ns = "default"
	}
	return &EnterpriseMeta{Namespace: ns, Partition: ap}
}

type Service struct {
	*EnterpriseMeta
	Name        string
	Port        int
	Datacenter  string
	TrustDomain string
	Subset      string
}

// ParseUpstream parses a string in unlabeled upstream format into a Service instance.
func ParseUpstream(s string) (Service, error) {
	var upstream Service
	var err error

	// Split on ":" to extract the name components, port and optional datacenter
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return upstream, fmt.Errorf("invalid service format: %s", s)
	}

	// Get the port
	if upstream.Port, err = strconv.Atoi(parts[1]); err != nil {
		return upstream, fmt.Errorf("invalid service port: %w", err)
	}

	// Split the first part on "." to get the qualified component parts
	qname := strings.Split(parts[0], ".")
	upstream.Name = qname[0]
	if len(qname) > 1 {
		upstream.EnterpriseMeta = &EnterpriseMeta{
			Namespace: qname[1],
			Partition: "default",
		}
	}
	if len(qname) > 2 {
		upstream.Partition = qname[2]
	}

	// Optional datacenter
	if len(parts) > 2 {
		upstream.Datacenter = parts[2]
	}

	return upstream, nil
}

func (s Service) SNI() string {
	ns := s.NamespaceOrDefault()
	ap := s.PartitionOrDefault()
	dc := s.DatacenterOrDefault()

	switch ap {
	case "default":
		if s.Subset == "" {
			return dotJoin(s.Name, ns, dc, internal, s.TrustDomain)
		} else {
			return dotJoin(s.Subset, s.Name, ns, dc, internal, s.TrustDomain)
		}
	default:
		if s.Subset == "" {
			return dotJoin(s.Name, ns, ap, dc, internalVersion, s.TrustDomain)
		} else {
			return dotJoin(s.Subset, s.Name, ns, ap, dc, internalVersion, s.TrustDomain)
		}
	}
}

func (s Service) DatacenterOrDefault() string {
	if s.Datacenter == "" {
		return "dc1"
	}
	return s.Datacenter
}

func (s Service) NamespaceOrDefault() string {
	if s.EnterpriseMeta != nil && s.Namespace != "" {
		return s.Namespace
	}
	return "default"
}

func (s Service) PartitionOrDefault() string {
	if s.EnterpriseMeta != nil && s.Partition != "" {
		return s.Partition
	}
	return "default"
}

func (s Service) SpiffeID() string {
	path := fmt.Sprintf("/ns/%s/dc/%s/svc/%s",
		s.NamespaceOrDefault(),
		s.DatacenterOrDefault(),
		s.Name,
	)

	// Although OSS has no support for partitions, it still needs to be able to
	// handle exportedPartition from peered Consul Enterprise clusters in order
	// to generate the correct SpiffeID.
	// We intentionally avoid using pbpartition.DefaultName here to be OSS friendly.
	if ap := s.PartitionOrDefault(); ap != "default" {
		path = "/ap/" + ap + path
	}

	id := &url.URL{
		Scheme: "spiffe",
		Host:   s.TrustDomain,
		Path:   path,
	}
	return id.String()
}

func (s Service) ExtensionPath() string {
	return fmt.Sprintf("/%s/%s/%s", s.PartitionOrDefault(), s.NamespaceOrDefault(), s.Name)
}

func dotJoin(parts ...string) string {
	return strings.Join(parts, ".")
}
