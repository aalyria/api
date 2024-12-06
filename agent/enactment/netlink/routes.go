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

func addressFamilyOfIP(ip *net.IP) int {
	switch ip != nil {
	case ip.To4() != nil:
		return unix.AF_INET
	case ip.To16() != nil:
		// It seems like net.IP lacks a proper test for IPv6, and that
		// `To4() == nil && To16() != nil` might be the only option?
		//
		// Sigh.
		return unix.AF_INET6
	}
	return unix.AF_UNSPEC
}

func addressFamilyOfIPNet(ipNet *net.IPNet) int {
	var ip *net.IP = nil
	if ipNet != nil {
		ip = &ipNet.IP
	}
	return addressFamilyOfIP(ip)
}

func makeIPNetForIP(ip *net.IP) *net.IPNet {
	switch addressFamilyOfIP(ip) {
	case unix.AF_INET:
		return &net.IPNet{IP: *ip, Mask: net.CIDRMask(32, 32)}
	case unix.AF_INET6:
		return &net.IPNet{IP: *ip, Mask: net.CIDRMask(128, 128)}
	default:
		return nil
	}
}

func newInstalledRoute(cfg Config, id string, cc *schedpb.SetRoute) (*installedRoute, error) {
	devID, err := cfg.GetLinkIDByName(cc.Dev)
	if err != nil {
		return nil, &OutInterfaceIdxError{wrongIface: cc.Dev, sourceError: err}
	}
	_, dst, err := net.ParseCIDR(cc.To)
	if err != nil || addressFamilyOfIPNet(dst) == unix.AF_UNSPEC {
		return nil, &IPFormattingError{ip: cc.To, sourceField: Dst_IPField}
	}
	nextHopIP, _, err := net.ParseCIDR(cc.Via)
	if err != nil || nextHopIP == nil {
		nextHopIP = net.ParseIP(cc.Via)
		if nextHopIP == nil {
			return nil, &IPFormattingError{ip: cc.Via, sourceField: Via_IPField}
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
	netlinkRoutes := []vnl.Route{}

	// If the next hop IP address is a link-local unicast address
	// (LLUA) there will already be a directly connected route for
	// the link-local prefix, i.e. fe80::/64 (or 169.254.0.0/16).
	if !i.Via.IsLinkLocalUnicast() {
		netlinkRoutes = append(netlinkRoutes, vnl.Route{
			LinkIndex: i.DevID,
			Scope:     vnl.SCOPE_LINK,
			Dst:       makeIPNetForIP(&i.Via),
			Protocol:  vnl.RouteProtocol(0),
			Family:    addressFamilyOfIP(&i.Via),
			Table:     cfg.RtTableID,
			Type:      unix.RTN_UNICAST,
		})
	}

	netlinkRoutes = append(netlinkRoutes, vnl.Route{
		LinkIndex: i.DevID,
		Scope:     vnl.SCOPE_UNIVERSE,
		Dst:       i.To,
		Gw:        i.Via,
		Protocol:  vnl.RouteProtocol(0),
		Family:    addressFamilyOfIPNet(i.To),
		Table:     cfg.RtTableID,
		Type:      unix.RTN_UNICAST,
	})

	return netlinkRoutes
}

func routesMostlyEqual(l, r vnl.Route) bool {
	return l.LinkIndex == r.LinkIndex &&
		l.Scope == r.Scope &&
		l.Table == r.Table &&
		((l.Src == nil && r.Src == nil) || l.Src.Equal(r.Src)) &&
		((l.Gw == nil && r.Gw == nil) || l.Gw.Equal(r.Gw)) &&
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
