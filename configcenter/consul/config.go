package consul

import "github.com/hashicorp/consul/api"

type Conf struct {
	Host       string        `json:",optional"`
	Scheme     string        `json:",default=http"`
	PathPrefix string        `json:",optional"`
	Datacenter string        `json:",optional"`
	Token      string        `json:",optional"`
	TLSConfig  api.TLSConfig `json:"TLSConfig,optional"`
	Key        string        `json:",optional"`
	Type       string        `json:",default=yaml,options=yaml|hcl|json|xml"`
}
