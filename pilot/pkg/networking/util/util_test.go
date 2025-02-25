// Copyright 2018 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"reflect"
	"testing"
	"time"

	v2 "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	listener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	http_conn "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	xdsutil "github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/wrappers"

	proto2 "istio.io/istio/pkg/proto"

	"github.com/golang/protobuf/proto"
	"gopkg.in/d4l3k/messagediff.v1"

	meshconfig "istio.io/api/mesh/v1alpha1"
	"istio.io/istio/pilot/pkg/model"
)

func TestConvertAddressToCidr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want *core.CidrRange
	}{
		{
			"return nil when the address is empty",
			"",
			nil,
		},
		{
			"success case with no PrefixLen",
			"1.2.3.4",
			&core.CidrRange{
				AddressPrefix: "1.2.3.4",
				PrefixLen: &wrappers.UInt32Value{
					Value: 32,
				},
			},
		},
		{
			"success case with PrefixLen",
			"1.2.3.4/16",
			&core.CidrRange{
				AddressPrefix: "1.2.3.4",
				PrefixLen: &wrappers.UInt32Value{
					Value: 16,
				},
			},
		},
		{
			"ipv6",
			"2001:db8::",
			&core.CidrRange{
				AddressPrefix: "2001:db8::",
				PrefixLen: &wrappers.UInt32Value{
					Value: 128,
				},
			},
		},
		{
			"ipv6 with prefix",
			"2001:db8::/64",
			&core.CidrRange{
				AddressPrefix: "2001:db8::",
				PrefixLen: &wrappers.UInt32Value{
					Value: 64,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertAddressToCidr(tt.addr); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertAddressToCidr() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetNetworkEndpointAddress(t *testing.T) {
	neUnix := &model.NetworkEndpoint{
		Family:  model.AddressFamilyUnix,
		Address: "/var/run/test/test.sock",
	}
	aUnix := GetNetworkEndpointAddress(neUnix)
	if aUnix.GetPipe() == nil {
		t.Fatalf("GetAddress() => want Pipe, got %s", aUnix.String())
	}
	if aUnix.GetPipe().GetPath() != neUnix.Address {
		t.Fatalf("GetAddress() => want path %s, got %s", neUnix.Address, aUnix.GetPipe().GetPath())
	}

	neIP := &model.NetworkEndpoint{
		Family:  model.AddressFamilyTCP,
		Address: "192.168.10.45",
		Port:    4558,
	}
	aIP := GetNetworkEndpointAddress(neIP)
	sock := aIP.GetSocketAddress()
	if sock == nil {
		t.Fatalf("GetAddress() => want SocketAddress, got %s", aIP.String())
	}
	if sock.GetAddress() != neIP.Address {
		t.Fatalf("GetAddress() => want %s, got %s", neIP.Address, sock.GetAddress())
	}
	if int(sock.GetPortValue()) != neIP.Port {
		t.Fatalf("GetAddress() => want port %d, got port %d", neIP.Port, sock.GetPortValue())
	}
}

func TestResolveHostsInNetworksConfig(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		modified bool
	}{
		{
			"Gateway with IP address",
			"9.142.3.1",
			false,
		},
		{
			"Gateway with localhost address",
			"localhost",
			true,
		},
		{
			"Gateway with empty address",
			"",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &meshconfig.MeshNetworks{
				Networks: map[string]*meshconfig.Network{
					"network": {
						Gateways: []*meshconfig.Network_IstioNetworkGateway{
							{
								Gw: &meshconfig.Network_IstioNetworkGateway_Address{
									Address: tt.address,
								},
							},
						},
					},
				},
			}
			ResolveHostsInNetworksConfig(config)
			addrAfter := config.Networks["network"].Gateways[0].GetAddress()
			if addrAfter == tt.address && tt.modified {
				t.Fatalf("Expected network address to be modified but it's the same as before calling the function")
			}
			if addrAfter != tt.address && !tt.modified {
				t.Fatalf("Expected network address not to be modified after calling the function")
			}
		})
	}
}

func TestConvertLocality(t *testing.T) {
	tests := []struct {
		name     string
		locality string
		want     *core.Locality
		reverse  string
	}{
		{
			name:     "nil locality",
			locality: "",
			want:     nil,
		},
		{
			name:     "locality with only region",
			locality: "region",
			want: &core.Locality{
				Region: "region",
			},
		},
		{
			name:     "locality with region and zone",
			locality: "region/zone",
			want: &core.Locality{
				Region: "region",
				Zone:   "zone",
			},
		},
		{
			name:     "locality with region zone and subzone",
			locality: "region/zone/subzone",
			want: &core.Locality{
				Region:  "region",
				Zone:    "zone",
				SubZone: "subzone",
			},
		},
		{
			name:     "locality with region zone subzone and rack",
			locality: "region/zone/subzone/rack",
			want: &core.Locality{
				Region:  "region",
				Zone:    "zone",
				SubZone: "subzone",
			},
			reverse: "region/zone/subzone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertLocality(tt.locality)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Expected locality %#v, but got %#v", tt.want, got)
			}
			// Verify we can reverse the conversion back to the original input
			reverse := LocalityToString(got)
			if tt.reverse != "" {
				// Special case, reverse lookup is different than original input
				if tt.reverse != reverse {
					t.Errorf("Expected locality string %s, got %v", tt.reverse, reverse)
				}
			} else if tt.locality != reverse {
				t.Errorf("Expected locality string %s, got %v", tt.locality, reverse)
			}
		})
	}
}

func TestLocalityMatch(t *testing.T) {
	tests := []struct {
		name     string
		locality *core.Locality
		rule     string
		match    bool
	}{
		{
			name: "wildcard matching",
			locality: &core.Locality{
				Region:  "region1",
				Zone:    "zone1",
				SubZone: "subzone1",
			},
			rule:  "*",
			match: true,
		},
		{
			name: "wildcard matching",
			locality: &core.Locality{
				Region:  "region1",
				Zone:    "zone1",
				SubZone: "subzone1",
			},
			rule:  "region1/*",
			match: true,
		},
		{
			name: "wildcard matching",
			locality: &core.Locality{
				Region:  "region1",
				Zone:    "zone1",
				SubZone: "subzone1",
			},
			rule:  "region1/zone1/*",
			match: true,
		},
		{
			name: "wildcard not matching",
			locality: &core.Locality{
				Region:  "region1",
				Zone:    "zone1",
				SubZone: "subzone1",
			},
			rule:  "region1/zone2/*",
			match: false,
		},
		{
			name: "region matching",
			locality: &core.Locality{
				Region:  "region1",
				Zone:    "zone1",
				SubZone: "subzone1",
			},
			rule:  "region1",
			match: true,
		},
		{
			name: "region and zone matching",
			locality: &core.Locality{
				Region:  "region1",
				Zone:    "zone1",
				SubZone: "subzone1",
			},
			rule:  "region1/zone1",
			match: true,
		},
		{
			name: "zubzone wildcard matching",
			locality: &core.Locality{
				Region: "region1",
				Zone:   "zone1",
			},
			rule:  "region1/zone1",
			match: true,
		},
		{
			name: "subzone mismatching",
			locality: &core.Locality{
				Region: "region1",
				Zone:   "zone1",
			},
			rule:  "region1/zone1/subzone2",
			match: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := LocalityMatch(tt.locality, tt.rule)
			if match != tt.match {
				t.Errorf("Expected matching result %v, but got %v", tt.match, match)
			}
		})
	}
}

func TestIsLocalityEmpty(t *testing.T) {
	tests := []struct {
		name     string
		locality *core.Locality
		want     bool
	}{
		{
			"non empty locality",
			&core.Locality{
				Region: "region",
			},
			false,
		},
		{
			"empty locality",
			&core.Locality{
				Region: "",
			},
			true,
		},
		{
			"nil locality",
			nil,
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsLocalityEmpty(tt.locality)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Expected locality empty result %#v, but got %#v", tt.want, got)
			}
		})
	}
}

func TestBuildConfigInfoMetadata(t *testing.T) {
	cases := []struct {
		name string
		in   model.ConfigMeta
		want *core.Metadata
	}{
		{
			"destination-rule",
			model.ConfigMeta{
				Group:     "networking.istio.io",
				Version:   "v1alpha3",
				Name:      "svcA",
				Namespace: "default",
				Domain:    "svc.cluster.local",
				Type:      "destination-rule",
			},
			&core.Metadata{
				FilterMetadata: map[string]*structpb.Struct{
					IstioMetadataKey: {
						Fields: map[string]*structpb.Value{
							"config": {
								Kind: &structpb.Value_StringValue{
									StringValue: "/apis/networking.istio.io/v1alpha3/namespaces/default/destination-rule/svcA",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, v := range cases {
		t.Run(v.name, func(tt *testing.T) {
			got := BuildConfigInfoMetadata(v.in)
			if diff, equal := messagediff.PrettyDiff(got, v.want); !equal {
				tt.Errorf("BuildConfigInfoMetadata(%v) produced incorrect result:\ngot: %v\nwant: %v\nDiff: %s", v.in, got, v.want, diff)
			}
		})
	}
}

func TestCloneCluster(t *testing.T) {
	cluster := buildFakeCluster()
	clone := CloneCluster(cluster)
	cluster.LoadAssignment.Endpoints[0].LoadBalancingWeight.Value = 10
	cluster.LoadAssignment.Endpoints[0].Priority = 8
	cluster.LoadAssignment.Endpoints[0].LbEndpoints = nil

	if clone.LoadAssignment.Endpoints[0].LoadBalancingWeight.GetValue() == 10 {
		t.Errorf("LoadBalancingWeight mutated")
	}
	if clone.LoadAssignment.Endpoints[0].Priority == 8 {
		t.Errorf("Priority mutated")
	}
	if clone.LoadAssignment.Endpoints[0].LbEndpoints == nil {
		t.Errorf("LbEndpoints mutated")
	}
}

func buildFakeCluster() *v2.Cluster {
	return &v2.Cluster{
		Name: "outbound|8080||test.example.org",
		LoadAssignment: &v2.ClusterLoadAssignment{
			ClusterName: "outbound|8080||test.example.org",
			Endpoints: []*endpoint.LocalityLbEndpoints{
				{
					Locality: &core.Locality{
						Region:  "region1",
						Zone:    "zone1",
						SubZone: "subzone1",
					},
					LbEndpoints: []*endpoint.LbEndpoint{},
					LoadBalancingWeight: &wrappers.UInt32Value{
						Value: 1,
					},
					Priority: 0,
				},
				{
					Locality: &core.Locality{
						Region:  "region1",
						Zone:    "zone1",
						SubZone: "subzone2",
					},
					LbEndpoints: []*endpoint.LbEndpoint{},
					LoadBalancingWeight: &wrappers.UInt32Value{
						Value: 1,
					},
					Priority: 0,
				},
			},
		},
	}
}

func TestIsHTTPFilterChain(t *testing.T) {
	httpFilterChain := &listener.FilterChain{
		Filters: []*listener.Filter{
			{
				Name: xdsutil.HTTPConnectionManager,
			},
		},
	}

	tcpFilterChain := &listener.FilterChain{
		Filters: []*listener.Filter{
			{
				Name: xdsutil.TCPProxy,
			},
		},
	}

	if !IsHTTPFilterChain(httpFilterChain) {
		t.Errorf("http Filter chain not detected properly")
	}

	if IsHTTPFilterChain(tcpFilterChain) {
		t.Errorf("tcp filter chain detected as http filter chain")
	}
}

var (
	listener80 = &v2.Listener{Address: BuildAddress("0.0.0.0", 80)}
	listener81 = &v2.Listener{Address: BuildAddress("0.0.0.0", 81)}
	listenerip = &v2.Listener{Address: BuildAddress("1.1.1.1", 80)}
)

func BenchmarkGetByAddress(b *testing.B) {
	for n := 0; n < b.N; n++ {
		GetByAddress([]*v2.Listener{
			listener80,
			listener81,
			listenerip,
		}, *listenerip.Address)
	}
}

func TestGetByAddress(t *testing.T) {
	tests := []struct {
		name      string
		listeners []*v2.Listener
		address   *core.Address
		expected  *v2.Listener
	}{
		{
			"no listeners",
			[]*v2.Listener{},
			BuildAddress("0.0.0.0", 80),
			nil,
		},
		{
			"single listener",
			[]*v2.Listener{
				listener80,
			},
			BuildAddress("0.0.0.0", 80),
			listener80,
		},
		{
			"multiple listeners",
			[]*v2.Listener{
				listener81,
				listenerip,
				listener80,
			},
			BuildAddress("0.0.0.0", 80),
			listener80,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetByAddress(tt.listeners, *tt.address)
			if got != tt.expected {
				t.Errorf("Got %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestMergeAnyWithStruct(t *testing.T) {
	inHCM := &http_conn.HttpConnectionManager{
		CodecType:  http_conn.HttpConnectionManager_HTTP1,
		StatPrefix: "123",
		HttpFilters: []*http_conn.HttpFilter{
			{
				Name: "filter1",
				ConfigType: &http_conn.HttpFilter_TypedConfig{
					TypedConfig: &any.Any{},
				},
			},
		},
		ServerName:        "scooby",
		XffNumTrustedHops: 2,
	}
	inAny := MessageToAny(inHCM)

	// listener.go sets this to 0
	newTimeout := ptypes.DurationProto(5 * time.Minute)
	userHCM := &http_conn.HttpConnectionManager{
		AddUserAgent:      proto2.BoolTrue,
		IdleTimeout:       newTimeout,
		StreamIdleTimeout: newTimeout,
		UseRemoteAddress:  proto2.BoolTrue,
		XffNumTrustedHops: 5,
		ServerName:        "foobar",
		HttpFilters: []*http_conn.HttpFilter{
			{
				Name: "some filter",
			},
		},
	}

	expectedHCM := proto.Clone(inHCM).(*http_conn.HttpConnectionManager)
	expectedHCM.AddUserAgent = userHCM.AddUserAgent
	expectedHCM.IdleTimeout = userHCM.IdleTimeout
	expectedHCM.StreamIdleTimeout = userHCM.StreamIdleTimeout
	expectedHCM.UseRemoteAddress = userHCM.UseRemoteAddress
	expectedHCM.XffNumTrustedHops = userHCM.XffNumTrustedHops
	expectedHCM.HttpFilters = append(expectedHCM.HttpFilters, userHCM.HttpFilters...)
	expectedHCM.ServerName = userHCM.ServerName

	pbStruct := MessageToStruct(userHCM)

	outAny, err := MergeAnyWithStruct(inAny, pbStruct)
	if err != nil {
		t.Errorf("Failed to merge: %v", err)
	}

	outHCM := http_conn.HttpConnectionManager{}
	if err = ptypes.UnmarshalAny(outAny, &outHCM); err != nil {
		t.Errorf("Failed to unmarshall outAny to outHCM: %v", err)
	}

	if !reflect.DeepEqual(expectedHCM, &outHCM) {
		t.Errorf("Merged HCM does not match the expected output")
	}
}
