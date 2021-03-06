// Copyright (C) 2016 Nippon Telegraph and Telephone Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package client provides a wrapper for GoBGP's gRPC API
package client

import (
	"fmt"
	"net"
	"strconv"
	"time"

	api "github.com/osrg/gobgp/api"
	"github.com/osrg/gobgp/config"
	"github.com/osrg/gobgp/packet/bgp"
	"github.com/osrg/gobgp/table"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

type GoBGPClient struct {
	conn *grpc.ClientConn
	cli  api.GobgpApiClient
}

func defaultGRPCOptions() []grpc.DialOption {
	return []grpc.DialOption{grpc.WithTimeout(time.Second), grpc.WithBlock(), grpc.WithInsecure()}
}

func NewGoBGPClient(target string, opts ...grpc.DialOption) (*GoBGPClient, error) {
	if target == "" {
		target = ":50051"
	}
	if len(opts) == 0 {
		opts = defaultGRPCOptions()
	}
	conn, err := grpc.Dial(target, opts...)
	if err != nil {
		return nil, err
	}
	cli := api.NewGobgpApiClient(conn)
	return &GoBGPClient{conn: conn, cli: cli}, nil
}

func (cli *GoBGPClient) Close() error {
	return cli.conn.Close()
}

func (cli *GoBGPClient) StartServer(c *config.Global) error {
	_, err := cli.cli.StartServer(context.Background(), &api.StartServerRequest{
		Global: &api.Global{
			As:               c.Config.As,
			RouterId:         c.Config.RouterId,
			ListenPort:       c.Config.Port,
			ListenAddresses:  c.Config.LocalAddressList,
			UseMultiplePaths: c.UseMultiplePaths.Config.Enabled,
		},
	})
	return err
}

func (cli *GoBGPClient) StopServer() error {
	_, err := cli.cli.StopServer(context.Background(), &api.StopServerRequest{})
	return err
}

func (cli *GoBGPClient) GetServer() (*config.Global, error) {
	ret, err := cli.cli.GetServer(context.Background(), &api.GetServerRequest{})
	if err != nil {
		return nil, err
	}
	return &config.Global{
		Config: config.GlobalConfig{
			As:               ret.Global.As,
			RouterId:         ret.Global.RouterId,
			Port:             ret.Global.ListenPort,
			LocalAddressList: ret.Global.ListenAddresses,
		},
		UseMultiplePaths: config.UseMultiplePaths{
			Config: config.UseMultiplePathsConfig{
				Enabled: ret.Global.UseMultiplePaths,
			},
		},
	}, nil
}

func (cli *GoBGPClient) getNeighbor(name string, afi int, vrf string) ([]*config.Neighbor, error) {
	ret, err := cli.cli.GetNeighbor(context.Background(), &api.GetNeighborRequest{EnableAdvertised: name != ""})
	if err != nil {
		return nil, err
	}

	neighbors := make([]*config.Neighbor, 0, len(ret.Peers))

	for _, p := range ret.Peers {
		if name != "" && name != p.Conf.NeighborAddress {
			continue
		}
		if vrf != "" && name != p.Conf.Vrf {
			continue
		}
		if afi > 0 {
			v6 := net.ParseIP(p.Conf.NeighborAddress).To4() == nil
			if afi == bgp.AFI_IP && v6 || afi == bgp.AFI_IP6 && !v6 {
				continue
			}
		}
		n, err := api.NewNeighborFromAPIStruct(p)
		if err != nil {
			return nil, err
		}
		neighbors = append(neighbors, n)
	}
	return neighbors, nil
}

func (cli *GoBGPClient) ListNeighbor() ([]*config.Neighbor, error) {
	return cli.getNeighbor("", 0, "")
}

func (cli *GoBGPClient) ListNeighborByTransport(afi int) ([]*config.Neighbor, error) {
	return cli.getNeighbor("", afi, "")
}

func (cli *GoBGPClient) ListNeighborByVRF(vrf string) ([]*config.Neighbor, error) {
	return cli.getNeighbor("", 0, vrf)
}

func (cli *GoBGPClient) GetNeighbor(name string) (*config.Neighbor, error) {
	ns, err := cli.getNeighbor(name, 0, "")
	if err != nil {
		return nil, err
	}
	if len(ns) == 0 {
		return nil, fmt.Errorf("not found neighbor %s", name)
	}
	return ns[0], nil
}

func (cli *GoBGPClient) AddNeighbor(c *config.Neighbor) error {
	peer := api.NewPeerFromConfigStruct(c)
	_, err := cli.cli.AddNeighbor(context.Background(), &api.AddNeighborRequest{Peer: peer})
	return err
}

func (cli *GoBGPClient) DeleteNeighbor(c *config.Neighbor) error {
	peer := api.NewPeerFromConfigStruct(c)
	_, err := cli.cli.DeleteNeighbor(context.Background(), &api.DeleteNeighborRequest{Peer: peer})
	return err
}

//func (cli *GoBGPClient) UpdateNeighbor(c *config.Neighbor) (bool, error) {
//}

func (cli *GoBGPClient) ShutdownNeighbor(addr string) error {
	_, err := cli.cli.ShutdownNeighbor(context.Background(), &api.ShutdownNeighborRequest{Address: addr})
	return err
}

func (cli *GoBGPClient) ResetNeighbor(addr string) error {
	_, err := cli.cli.ResetNeighbor(context.Background(), &api.ResetNeighborRequest{Address: addr})
	return err
}

func (cli *GoBGPClient) EnableNeighbor(addr string) error {
	_, err := cli.cli.EnableNeighbor(context.Background(), &api.EnableNeighborRequest{Address: addr})
	return err
}

func (cli *GoBGPClient) DisableNeighbor(addr string) error {
	_, err := cli.cli.DisableNeighbor(context.Background(), &api.DisableNeighborRequest{Address: addr})
	return err
}

func (cli *GoBGPClient) softreset(addr string, family bgp.RouteFamily, dir api.SoftResetNeighborRequest_SoftResetDirection) error {
	_, err := cli.cli.SoftResetNeighbor(context.Background(), &api.SoftResetNeighborRequest{
		Address:   addr,
		Direction: dir,
	})
	return err
}

func (cli *GoBGPClient) SoftResetIn(addr string, family bgp.RouteFamily) error {
	return cli.softreset(addr, family, api.SoftResetNeighborRequest_IN)
}

func (cli *GoBGPClient) SoftResetOut(addr string, family bgp.RouteFamily) error {
	return cli.softreset(addr, family, api.SoftResetNeighborRequest_OUT)
}

func (cli *GoBGPClient) SoftReset(addr string, family bgp.RouteFamily) error {
	return cli.softreset(addr, family, api.SoftResetNeighborRequest_BOTH)
}

func (cli *GoBGPClient) getRIB(resource api.Resource, name string, family bgp.RouteFamily, prefixes []*table.LookupPrefix) (*table.Table, error) {
	dsts := make([]*api.Destination, 0, len(prefixes))
	for _, p := range prefixes {
		longer := false
		shorter := false
		if p.LookupOption&table.LOOKUP_LONGER > 0 {
			longer = true
		}
		if p.LookupOption&table.LOOKUP_SHORTER > 0 {
			shorter = true
		}
		dsts = append(dsts, &api.Destination{
			Prefix:          p.Prefix,
			LongerPrefixes:  longer,
			ShorterPrefixes: shorter,
		})
	}
	res, err := cli.cli.GetRib(context.Background(), &api.GetRibRequest{
		Table: &api.Table{
			Type:         resource,
			Family:       uint32(family),
			Name:         name,
			Destinations: dsts,
		},
	})
	if err != nil {
		return nil, err
	}
	return res.Table.ToNativeTable()
}

func (cli *GoBGPClient) GetRIB(family bgp.RouteFamily, prefixes []*table.LookupPrefix) (*table.Table, error) {
	return cli.getRIB(api.Resource_GLOBAL, "", family, prefixes)
}

func (cli *GoBGPClient) GetLocalRIB(name string, family bgp.RouteFamily, prefixes []*table.LookupPrefix) (*table.Table, error) {
	return cli.getRIB(api.Resource_LOCAL, name, family, prefixes)
}

func (cli *GoBGPClient) GetAdjRIBIn(name string, family bgp.RouteFamily, prefixes []*table.LookupPrefix) (*table.Table, error) {
	return cli.getRIB(api.Resource_ADJ_IN, name, family, prefixes)
}

func (cli *GoBGPClient) GetAdjRIBOut(name string, family bgp.RouteFamily, prefixes []*table.LookupPrefix) (*table.Table, error) {
	return cli.getRIB(api.Resource_ADJ_OUT, name, family, prefixes)
}

func (cli *GoBGPClient) GetVRFRIB(name string, family bgp.RouteFamily, prefixes []*table.LookupPrefix) (*table.Table, error) {
	return cli.getRIB(api.Resource_VRF, name, family, prefixes)
}

func (cli *GoBGPClient) getRIBInfo(resource api.Resource, name string, family bgp.RouteFamily) (*table.TableInfo, error) {
	res, err := cli.cli.GetRibInfo(context.Background(), &api.GetRibInfoRequest{
		Info: &api.TableInfo{
			Type:   resource,
			Name:   name,
			Family: uint32(family),
		},
	})
	if err != nil {
		return nil, err
	}
	return &table.TableInfo{
		NumDestination: int(res.Info.NumDestination),
		NumPath:        int(res.Info.NumPath),
		NumAccepted:    int(res.Info.NumAccepted),
	}, nil

}

func (cli *GoBGPClient) GetRIBInfo(family bgp.RouteFamily) (*table.TableInfo, error) {
	return cli.getRIBInfo(api.Resource_GLOBAL, "", family)
}

func (cli *GoBGPClient) GetLocalRIBInfo(name string, family bgp.RouteFamily) (*table.TableInfo, error) {
	return cli.getRIBInfo(api.Resource_LOCAL, name, family)
}

func (cli *GoBGPClient) GetAdjRIBInInfo(name string, family bgp.RouteFamily) (*table.TableInfo, error) {
	return cli.getRIBInfo(api.Resource_ADJ_IN, name, family)
}

func (cli *GoBGPClient) GetAdjRIBOutInfo(name string, family bgp.RouteFamily) (*table.TableInfo, error) {
	return cli.getRIBInfo(api.Resource_ADJ_OUT, name, family)
}

type AddPathByStreamClient struct {
	stream api.GobgpApi_InjectMrtClient
}

func (c *AddPathByStreamClient) Send(paths ...*table.Path) error {
	ps := make([]*api.Path, 0, len(paths))
	for _, p := range paths {
		ps = append(ps, api.ToPathApi(p))
	}
	return c.stream.Send(&api.InjectMrtRequest{
		Resource: api.Resource_GLOBAL,
		Paths:    ps,
	})
}

func (c *AddPathByStreamClient) Close() error {
	_, err := c.stream.CloseAndRecv()
	return err
}

func (cli *GoBGPClient) AddPathByStream() (*AddPathByStreamClient, error) {
	stream, err := cli.cli.InjectMrt(context.Background())
	if err != nil {
		return nil, err
	}
	return &AddPathByStreamClient{stream}, nil
}

func (cli *GoBGPClient) addPath(vrfID string, pathList []*table.Path) ([]byte, error) {
	resource := api.Resource_GLOBAL
	if vrfID != "" {
		resource = api.Resource_VRF
	}
	var uuid []byte
	for _, path := range pathList {
		r, err := cli.cli.AddPath(context.Background(), &api.AddPathRequest{
			Resource: resource,
			VrfId:    vrfID,
			Path:     api.ToPathApi(path),
		})
		if err != nil {
			return nil, err
		}
		uuid = r.Uuid
	}
	return uuid, nil
}

func (cli *GoBGPClient) AddPath(pathList []*table.Path) ([]byte, error) {
	return cli.addPath("", pathList)
}

func (cli *GoBGPClient) AddVRFPath(vrfID string, pathList []*table.Path) ([]byte, error) {
	if vrfID == "" {
		return nil, fmt.Errorf("VRF ID is empty")
	}
	return cli.addPath(vrfID, pathList)
}

func (cli *GoBGPClient) deletePath(uuid []byte, f bgp.RouteFamily, vrfID string, pathList []*table.Path) error {
	var reqs []*api.DeletePathRequest

	resource := api.Resource_GLOBAL
	if vrfID != "" {
		resource = api.Resource_VRF
	}
	switch {
	case len(pathList) != 0:
		for _, path := range pathList {
			nlri := path.GetNlri()
			n, err := nlri.Serialize()
			if err != nil {
				return err
			}
			p := &api.Path{
				Nlri:   n,
				Family: uint32(path.GetRouteFamily()),
			}
			reqs = append(reqs, &api.DeletePathRequest{
				Resource: resource,
				VrfId:    vrfID,
				Path:     p,
			})
		}
	default:
		reqs = append(reqs, &api.DeletePathRequest{
			Resource: resource,
			VrfId:    vrfID,
			Uuid:     uuid,
			Family:   uint32(f),
		})
	}

	for _, req := range reqs {
		if _, err := cli.cli.DeletePath(context.Background(), req); err != nil {
			return err
		}
	}
	return nil
}

func (cli *GoBGPClient) DeletePath(pathList []*table.Path) error {
	return cli.deletePath(nil, bgp.RouteFamily(0), "", pathList)
}

func (cli *GoBGPClient) DeleteVRFPath(vrfID string, pathList []*table.Path) error {
	if vrfID == "" {
		return fmt.Errorf("VRF ID is empty")
	}
	return cli.deletePath(nil, bgp.RouteFamily(0), vrfID, pathList)
}

func (cli *GoBGPClient) DeletePathByUUID(uuid []byte) error {
	return cli.deletePath(uuid, bgp.RouteFamily(0), "", nil)
}

func (cli *GoBGPClient) DeletePathByFamily(family bgp.RouteFamily) error {
	return cli.deletePath(nil, family, "", nil)
}

func (cli *GoBGPClient) GetVRF() ([]*table.Vrf, error) {
	ret, err := cli.cli.GetVrf(context.Background(), &api.GetVrfRequest{})
	if err != nil {
		return nil, err
	}
	var vrfs []*table.Vrf

	f := func(bufs [][]byte) ([]bgp.ExtendedCommunityInterface, error) {
		ret := make([]bgp.ExtendedCommunityInterface, 0, len(bufs))
		for _, rt := range bufs {
			r, err := bgp.ParseExtended(rt)
			if err != nil {
				return nil, err
			}
			ret = append(ret, r)
		}
		return ret, nil
	}

	for _, vrf := range ret.Vrfs {
		importRT, err := f(vrf.ImportRt)
		if err != nil {
			return nil, err
		}
		exportRT, err := f(vrf.ExportRt)
		if err != nil {
			return nil, err
		}
		vrfs = append(vrfs, &table.Vrf{
			Name:     vrf.Name,
			Id:       vrf.Id,
			Rd:       bgp.GetRouteDistinguisher(vrf.Rd),
			ImportRt: importRT,
			ExportRt: exportRT,
		})
	}

	return vrfs, nil
}

func (cli *GoBGPClient) AddVRF(name string, id int, rd bgp.RouteDistinguisherInterface, im, ex []bgp.ExtendedCommunityInterface) error {
	buf, err := rd.Serialize()
	if err != nil {
		return err
	}

	f := func(comms []bgp.ExtendedCommunityInterface) ([][]byte, error) {
		var bufs [][]byte
		for _, c := range comms {
			buf, err := c.Serialize()
			if err != nil {
				return nil, err
			}
			bufs = append(bufs, buf)
		}
		return bufs, err
	}

	importRT, err := f(im)
	if err != nil {
		return err
	}
	exportRT, err := f(ex)
	if err != nil {
		return err
	}

	arg := &api.AddVrfRequest{
		Vrf: &api.Vrf{
			Name:     name,
			Rd:       buf,
			Id:       uint32(id),
			ImportRt: importRT,
			ExportRt: exportRT,
		},
	}
	_, err = cli.cli.AddVrf(context.Background(), arg)
	return err
}

func (cli *GoBGPClient) DeleteVRF(name string) error {
	arg := &api.DeleteVrfRequest{
		Vrf: &api.Vrf{
			Name: name,
		},
	}
	_, err := cli.cli.DeleteVrf(context.Background(), arg)
	return err
}

func (cli *GoBGPClient) GetDefinedSet(typ table.DefinedType) ([]table.DefinedSet, error) {
	ret, err := cli.cli.GetDefinedSet(context.Background(), &api.GetDefinedSetRequest{Type: api.DefinedType(typ)})
	if err != nil {
		return nil, err
	}
	ds := make([]table.DefinedSet, 0, len(ret.Sets))
	for _, s := range ret.Sets {
		d, err := api.NewDefinedSetFromApiStruct(s)
		if err != nil {
			return nil, err
		}
		ds = append(ds, d)
	}
	return ds, nil
}

func (cli *GoBGPClient) AddDefinedSet(d table.DefinedSet) error {
	a, err := api.NewAPIDefinedSetFromTableStruct(d)
	if err != nil {
		return err
	}
	_, err = cli.cli.AddDefinedSet(context.Background(), &api.AddDefinedSetRequest{
		Set: a,
	})
	return err
}

func (cli *GoBGPClient) DeleteDefinedSet(d table.DefinedSet, all bool) error {
	a, err := api.NewAPIDefinedSetFromTableStruct(d)
	if err != nil {
		return err
	}
	_, err = cli.cli.DeleteDefinedSet(context.Background(), &api.DeleteDefinedSetRequest{
		Set: a,
		All: all,
	})
	return err
}

func (cli *GoBGPClient) ReplaceDefinedSet(d table.DefinedSet) error {
	a, err := api.NewAPIDefinedSetFromTableStruct(d)
	if err != nil {
		return err
	}
	_, err = cli.cli.ReplaceDefinedSet(context.Background(), &api.ReplaceDefinedSetRequest{
		Set: a,
	})
	return err
}

func (cli *GoBGPClient) GetStatement() ([]*table.Statement, error) {
	ret, err := cli.cli.GetStatement(context.Background(), &api.GetStatementRequest{})
	if err != nil {
		return nil, err
	}
	sts := make([]*table.Statement, 0, len(ret.Statements))
	for _, s := range ret.Statements {
		st, err := api.NewStatementFromApiStruct(s)
		if err != nil {
			return nil, err
		}
		sts = append(sts, st)
	}
	return sts, nil
}

func (cli *GoBGPClient) AddStatement(t *table.Statement) error {
	a := api.NewAPIStatementFromTableStruct(t)
	_, err := cli.cli.AddStatement(context.Background(), &api.AddStatementRequest{
		Statement: a,
	})
	return err
}

func (cli *GoBGPClient) DeleteStatement(t *table.Statement, all bool) error {
	a := api.NewAPIStatementFromTableStruct(t)
	_, err := cli.cli.DeleteStatement(context.Background(), &api.DeleteStatementRequest{
		Statement: a,
		All:       all,
	})
	return err
}

func (cli *GoBGPClient) ReplaceStatement(t *table.Statement) error {
	a := api.NewAPIStatementFromTableStruct(t)
	_, err := cli.cli.ReplaceStatement(context.Background(), &api.ReplaceStatementRequest{
		Statement: a,
	})
	return err
}

func (cli *GoBGPClient) GetPolicy() ([]*table.Policy, error) {
	ret, err := cli.cli.GetPolicy(context.Background(), &api.GetPolicyRequest{})
	if err != nil {
		return nil, err
	}
	pols := make([]*table.Policy, 0, len(ret.Policies))
	for _, p := range ret.Policies {
		pol, err := api.NewPolicyFromApiStruct(p)
		if err != nil {
			return nil, err
		}
		pols = append(pols, pol)
	}
	return pols, nil
}

func (cli *GoBGPClient) AddPolicy(t *table.Policy, refer bool) error {
	a := api.NewAPIPolicyFromTableStruct(t)
	_, err := cli.cli.AddPolicy(context.Background(), &api.AddPolicyRequest{
		Policy:                  a,
		ReferExistingStatements: refer,
	})
	return err
}

func (cli *GoBGPClient) DeletePolicy(t *table.Policy, all, preserve bool) error {
	a := api.NewAPIPolicyFromTableStruct(t)
	_, err := cli.cli.DeletePolicy(context.Background(), &api.DeletePolicyRequest{
		Policy:             a,
		All:                all,
		PreserveStatements: preserve,
	})
	return err
}

func (cli *GoBGPClient) ReplacePolicy(t *table.Policy, refer, preserve bool) error {
	a := api.NewAPIPolicyFromTableStruct(t)
	_, err := cli.cli.ReplacePolicy(context.Background(), &api.ReplacePolicyRequest{
		Policy:                  a,
		ReferExistingStatements: refer,
		PreserveStatements:      preserve,
	})
	return err
}

func (cli *GoBGPClient) getPolicyAssignment(name string, dir table.PolicyDirection) (*table.PolicyAssignment, error) {
	var typ api.PolicyType
	switch dir {
	case table.POLICY_DIRECTION_IN:
		typ = api.PolicyType_IN
	case table.POLICY_DIRECTION_IMPORT:
		typ = api.PolicyType_IMPORT
	case table.POLICY_DIRECTION_EXPORT:
		typ = api.PolicyType_EXPORT
	}
	resource := api.Resource_GLOBAL
	if name != "" {
		resource = api.Resource_LOCAL
	}

	ret, err := cli.cli.GetPolicyAssignment(context.Background(), &api.GetPolicyAssignmentRequest{
		Assignment: &api.PolicyAssignment{
			Name:     name,
			Resource: resource,
			Type:     typ,
		},
	})
	if err != nil {
		return nil, err
	}

	def := table.ROUTE_TYPE_ACCEPT
	if ret.Assignment.Default == api.RouteAction_REJECT {
		def = table.ROUTE_TYPE_REJECT
	}

	pols := make([]*table.Policy, 0, len(ret.Assignment.Policies))
	for _, p := range ret.Assignment.Policies {
		pol, err := api.NewPolicyFromApiStruct(p)
		if err != nil {
			return nil, err
		}
		pols = append(pols, pol)
	}
	return &table.PolicyAssignment{
		Name:     name,
		Type:     dir,
		Policies: pols,
		Default:  def,
	}, nil
}

func (cli *GoBGPClient) GetImportPolicy() (*table.PolicyAssignment, error) {
	return cli.getPolicyAssignment("", table.POLICY_DIRECTION_IMPORT)
}

func (cli *GoBGPClient) GetExportPolicy() (*table.PolicyAssignment, error) {
	return cli.getPolicyAssignment("", table.POLICY_DIRECTION_EXPORT)
}

func (cli *GoBGPClient) GetRouteServerInPolicy(name string) (*table.PolicyAssignment, error) {
	return cli.getPolicyAssignment(name, table.POLICY_DIRECTION_IN)
}

func (cli *GoBGPClient) GetRouteServerImportPolicy(name string) (*table.PolicyAssignment, error) {
	return cli.getPolicyAssignment(name, table.POLICY_DIRECTION_IMPORT)
}

func (cli *GoBGPClient) GetRouteServerExportPolicy(name string) (*table.PolicyAssignment, error) {
	return cli.getPolicyAssignment(name, table.POLICY_DIRECTION_EXPORT)
}

func (cli *GoBGPClient) AddPolicyAssignment(assignment *table.PolicyAssignment) error {
	_, err := cli.cli.AddPolicyAssignment(context.Background(), &api.AddPolicyAssignmentRequest{
		Assignment: api.NewAPIPolicyAssignmentFromTableStruct(assignment),
	})
	return err
}

func (cli *GoBGPClient) DeletePolicyAssignment(assignment *table.PolicyAssignment, all bool) error {
	a := api.NewAPIPolicyAssignmentFromTableStruct(assignment)
	_, err := cli.cli.DeletePolicyAssignment(context.Background(), &api.DeletePolicyAssignmentRequest{
		Assignment: a,
		All:        all})
	return err
}

func (cli *GoBGPClient) ReplacePolicyAssignment(assignment *table.PolicyAssignment) error {
	_, err := cli.cli.ReplacePolicyAssignment(context.Background(), &api.ReplacePolicyAssignmentRequest{
		Assignment: api.NewAPIPolicyAssignmentFromTableStruct(assignment),
	})
	return err
}

//func (cli *GoBGPClient) EnableMrt(c *config.MrtConfig) error {
//}
//
//func (cli *GoBGPClient) DisableMrt(c *config.MrtConfig) error {
//}
//

func (cli *GoBGPClient) GetRPKI() ([]*config.RpkiServer, error) {
	rsp, err := cli.cli.GetRpki(context.Background(), &api.GetRpkiRequest{})
	if err != nil {
		return nil, err
	}
	servers := make([]*config.RpkiServer, 0, len(rsp.Servers))
	for _, s := range rsp.Servers {
		port, err := strconv.Atoi(s.Conf.RemotePort)
		if err != nil {
			return nil, err
		}
		server := &config.RpkiServer{
			Config: config.RpkiServerConfig{
				Address: s.Conf.Address,
				Port:    uint32(port),
			},
			State: config.RpkiServerState{
				Up:           s.State.Up,
				SerialNumber: s.State.Serial,
				RecordsV4:    s.State.RecordIpv4,
				RecordsV6:    s.State.RecordIpv6,
				PrefixesV4:   s.State.PrefixIpv4,
				PrefixesV6:   s.State.PrefixIpv6,
				Uptime:       s.State.Uptime,
				Downtime:     s.State.Downtime,
				RpkiMessages: config.RpkiMessages{
					RpkiReceived: config.RpkiReceived{
						SerialNotify:  s.State.SerialNotify,
						CacheReset:    s.State.CacheReset,
						CacheResponse: s.State.CacheResponse,
						Ipv4Prefix:    s.State.ReceivedIpv4,
						Ipv6Prefix:    s.State.ReceivedIpv6,
						EndOfData:     s.State.EndOfData,
						Error:         s.State.Error,
					},
					RpkiSent: config.RpkiSent{
						SerialQuery: s.State.SerialQuery,
						ResetQuery:  s.State.ResetQuery,
					},
				},
			},
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func (cli *GoBGPClient) GetROA(family bgp.RouteFamily) ([]*table.ROA, error) {
	rsp, err := cli.cli.GetRoa(context.Background(), &api.GetRoaRequest{
		Family: uint32(family),
	})
	if err != nil {
		return nil, err
	}
	roas := make([]*table.ROA, 0, len(rsp.Roas))
	for _, r := range rsp.Roas {
		ip := net.ParseIP(r.Prefix)
		if ip.To4() != nil {
			ip = ip.To4()
		}
		afi, _ := bgp.RouteFamilyToAfiSafi(family)
		roa := table.NewROA(int(afi), []byte(ip), uint8(r.Prefixlen), uint8(r.Maxlen), r.As, net.JoinHostPort(r.Conf.Address, r.Conf.RemotePort))
		roas = append(roas, roa)
	}
	return roas, nil
}

func (cli *GoBGPClient) AddRPKIServer(address string, port, lifetime int) error {
	_, err := cli.cli.AddRpki(context.Background(), &api.AddRpkiRequest{
		Address:  address,
		Port:     uint32(port),
		Lifetime: int64(lifetime),
	})
	return err
}

func (cli *GoBGPClient) DeleteRPKIServer(address string) error {
	_, err := cli.cli.DeleteRpki(context.Background(), &api.DeleteRpkiRequest{
		Address: address,
	})
	return err
}

func (cli *GoBGPClient) EnableRPKIServer(address string) error {
	_, err := cli.cli.EnableRpki(context.Background(), &api.EnableRpkiRequest{
		Address: address,
	})
	return err
}

func (cli *GoBGPClient) DisableRPKIServer(address string) error {
	_, err := cli.cli.DisableRpki(context.Background(), &api.DisableRpkiRequest{
		Address: address,
	})
	return err
}

func (cli *GoBGPClient) ResetRPKIServer(address string) error {
	_, err := cli.cli.ResetRpki(context.Background(), &api.ResetRpkiRequest{
		Address: address,
	})
	return err
}

func (cli *GoBGPClient) SoftResetRPKIServer(address string) error {
	_, err := cli.cli.SoftResetRpki(context.Background(), &api.SoftResetRpkiRequest{
		Address: address,
	})
	return err
}

func (cli *GoBGPClient) ValidateRIBWithRPKI(prefixes ...string) error {
	req := &api.ValidateRibRequest{}
	if len(prefixes) > 1 {
		return fmt.Errorf("too many prefixes: %d", len(prefixes))
	} else if len(prefixes) == 1 {
		req.Prefix = prefixes[0]
	}
	_, err := cli.cli.ValidateRib(context.Background(), req)
	return err
}

func (cli *GoBGPClient) AddBMP(c *config.BmpServerConfig) error {
	_, err := cli.cli.AddBmp(context.Background(), &api.AddBmpRequest{
		Address: c.Address,
		Port:    c.Port,
		Type:    api.AddBmpRequest_MonitoringPolicy(c.RouteMonitoringPolicy.ToInt()),
	})
	return err
}

func (cli *GoBGPClient) DeleteBMP(c *config.BmpServerConfig) error {
	_, err := cli.cli.DeleteBmp(context.Background(), &api.DeleteBmpRequest{
		Address: c.Address,
		Port:    c.Port,
	})
	return err
}

type MonitorRIBClient struct {
	stream api.GobgpApi_MonitorRibClient
}

func (c *MonitorRIBClient) Recv() (*table.Destination, error) {
	d, err := c.stream.Recv()
	if err != nil {
		return nil, err
	}
	return d.ToNativeDestination()
}

func (cli *GoBGPClient) MonitorRIB(family bgp.RouteFamily) (*MonitorRIBClient, error) {
	stream, err := cli.cli.MonitorRib(context.Background(), &api.Table{
		Type:   api.Resource_GLOBAL,
		Family: uint32(family),
	})
	if err != nil {
		return nil, err
	}
	return &MonitorRIBClient{stream}, nil
}

func (cli *GoBGPClient) MonitorAdjRIBIn(name string, family bgp.RouteFamily) (*MonitorRIBClient, error) {
	stream, err := cli.cli.MonitorRib(context.Background(), &api.Table{
		Type:   api.Resource_ADJ_IN,
		Name:   name,
		Family: uint32(family),
	})
	if err != nil {
		return nil, err
	}
	return &MonitorRIBClient{stream}, nil
}

type MonitorNeighborStateClient struct {
	stream api.GobgpApi_MonitorPeerStateClient
}

func (c *MonitorNeighborStateClient) Recv() (*config.Neighbor, error) {
	p, err := c.stream.Recv()
	if err != nil {
		return nil, err
	}
	return api.NewNeighborFromAPIStruct(p)
}

func (cli *GoBGPClient) MonitorNeighborState(names ...string) (*MonitorNeighborStateClient, error) {
	if len(names) > 1 {
		return nil, fmt.Errorf("support one name at most: %d", len(names))
	}
	name := ""
	if len(names) > 0 {
		name = names[0]
	}
	stream, err := cli.cli.MonitorPeerState(context.Background(), &api.Arguments{
		Name: name,
	})
	if err != nil {
		return nil, err
	}
	return &MonitorNeighborStateClient{stream}, nil
}
