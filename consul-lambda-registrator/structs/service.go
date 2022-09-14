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

type Service struct {
	Name        string
	Port        int
	Namespace   string
	Partition   string
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
		upstream.Namespace = qname[1]
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

func (u Service) SNI() string {
	ns := u.NamespaceOrDefault()
	ap := u.PartitionOrDefault()
	dc := u.DatacenterOrDefault()

	switch ap {
	case "default":
		if u.Subset == "" {
			return dotJoin(u.Name, ns, dc, internal, u.TrustDomain)
		} else {
			return dotJoin(u.Subset, u.Name, ns, dc, internal, u.TrustDomain)
		}
	default:
		if u.Subset == "" {
			return dotJoin(u.Name, ns, ap, dc, internalVersion, u.TrustDomain)
		} else {
			return dotJoin(u.Subset, u.Name, ns, ap, dc, internalVersion, u.TrustDomain)
		}
	}
}

func (u Service) DatacenterOrDefault() string {
	if u.Datacenter == "" {
		return "dc1"
	}
	return u.Datacenter
}

func (u Service) NamespaceOrDefault() string {
	if u.Namespace == "" {
		return "default"
	}
	return u.Namespace
}

func (u Service) PartitionOrDefault() string {
	if u.Partition == "" {
		return "default"
	}
	return u.Partition
}

func (u Service) SpiffeID() string {
	path := fmt.Sprintf("/ns/%s/dc/%s/svc/%s",
		u.NamespaceOrDefault(),
		u.DatacenterOrDefault(),
		u.Name,
	)

	// Although OSS has no support for partitions, it still needs to be able to
	// handle exportedPartition from peered Consul Enterprise clusters in order
	// to generate the correct SpiffeID.
	// We intentionally avoid using pbpartition.DefaultName here to be OSS friendly.
	if ap := u.PartitionOrDefault(); ap != "" && ap != "default" {
		path = "/ap/" + ap + path
	}

	id := &url.URL{
		Scheme: "spiffe",
		Host:   u.TrustDomain,
		Path:   path,
	}
	return id.String()
}

func (u Service) ExtensionPath() string {
	return fmt.Sprintf("/%s/%s/%s", u.PartitionOrDefault(), u.NamespaceOrDefault(), u.Name)
}

func dotJoin(parts ...string) string {
	return strings.Join(parts, ".")
}
