// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
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

package netlink

import (
	"bytes"
	"net"

	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	vnl "github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// TODO: this is all heavily IPv4-only; figure out IPv6 support.
// N.B. IPv6 routes with IPv4 next hops (and vice versa) should be
// considered as well (see RFC 9229).

type installedRoute struct {
	ID      string
	DevName string
	DevID   int
	To      *net.IPNet
	Via     net.IP
}

func newInstalledRoute(cfg Config, id string, cc *schedpb.SetRoute) (*installedRoute, error) {
	devID, err := cfg.GetLinkIDByName(cc.Dev)
	if err != nil {
		return nil, &OutInterfaceIdxError{wrongIface: cc.Dev, sourceError: err}
	}
	_, dst, err := net.ParseCIDR(cc.To)
	if err != nil || dst.IP.To4() == nil {
		return nil, &IPv4FormattingError{ipv4: cc.To, sourceField: Dst_IPv4Field}
	}
	nextHopIP, _, err := net.ParseCIDR(cc.Via)
	if err != nil || nextHopIP == nil {
		nextHopIP = net.ParseIP(cc.Via)
		if nextHopIP == nil {
			return nil, &IPv4FormattingError{ipv4: cc.Via, sourceField: Via_IPv4Field}
		}
	}

	return &installedRoute{
		ID:      id,
		DevName: cc.Dev,
		DevID:   devID,
		To:      dst,
		Via:     nextHopIP,
	}, nil
}

func (i *installedRoute) ToNetlinkRoutes(cfg Config) []vnl.Route {
	routeToGateway := vnl.Route{
		LinkIndex: i.DevID,
		Scope:     vnl.SCOPE_LINK,
		Dst:       &net.IPNet{IP: i.Via, Mask: net.CIDRMask(32, 32)},
		Protocol:  vnl.RouteProtocol(0),
		Family:    unix.AF_INET,
		Table:     cfg.RtTableID,
		Type:      unix.RTN_UNICAST,
	}

	routeToDst := vnl.Route{
		LinkIndex: i.DevID,
		Scope:     vnl.SCOPE_UNIVERSE,
		Dst:       i.To,
		Gw:        i.Via,
		Protocol:  vnl.RouteProtocol(0),
		Family:    unix.AF_INET,
		Table:     cfg.RtTableID,
		Type:      unix.RTN_UNICAST,
	}

	return []vnl.Route{routeToGateway, routeToDst}
}

func routesMostlyEqual(l, r vnl.Route) bool {
	return l.LinkIndex == r.LinkIndex &&
		l.Scope == r.Scope &&
		l.Src.Equal(r.Src) &&
		l.Gw.Equal(r.Gw) &&
		l.Dst.IP.Equal(r.Dst.IP) &&
		bytes.Equal(l.Dst.Mask, r.Dst.Mask)
}

func diffRoutes(got, want []vnl.Route) (toAdd []vnl.Route, toRemove []vnl.Route) {
	toKeep := map[int]struct{}{}

	for _, w := range want {
		found := false
		for gotID, g := range got {
			if found = routesMostlyEqual(w, g); found {
				toKeep[gotID] = struct{}{}
				break
			}
		}
		if !found {
			toAdd = append(toAdd, w)
		}
	}

	for gotID, g := range got {
		if _, ok := toKeep[gotID]; !ok {
			toRemove = append(toRemove, g)
		}
	}

	return toAdd, toRemove
}
