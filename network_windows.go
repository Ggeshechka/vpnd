//go:build windows
package main

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"time"

	"github.com/xtls/xray-core/app/router"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
	"google.golang.org/protobuf/proto"
)

const (
	tunName    = "xray0"
	vpsAddress = "de.safelane.pro"
)

func getPhysicalGateway() (winipcfg.LUID, netip.Addr, error) {
	routes, err := winipcfg.GetIPForwardTable2(windows.AF_INET)
	if err != nil {
		return 0, netip.Addr{}, err
	}

	var bestLUID winipcfg.LUID
	var bestNextHop netip.Addr
	var lowestMetric uint32 = ^uint32(0)

	for _, r := range routes {
		if r.DestinationPrefix.PrefixLength == 0 && r.Metric < lowestMetric {
			lowestMetric = r.Metric
			bestLUID = r.InterfaceLUID
			bestNextHop = r.NextHop.Addr()
		}
	}

	if lowestMetric == ^uint32(0) {
		return 0, netip.Addr{}, fmt.Errorf("шлюз не найден")
	}
	return bestLUID, bestNextHop, nil
}

func getRURoutes() ([]netip.Prefix, error) {
	data, err := os.ReadFile("geoip.dat")
	if err != nil {
		return nil, err
	}

	var geoipList router.GeoIPList
	if err := proto.Unmarshal(data, &geoipList); err != nil {
		return nil, err
	}

	var prefixes []netip.Prefix
	for _, geoip := range geoipList.Entry {
		if geoip.CountryCode == "RU" {
			for _, cidr := range geoip.Cidr {
				if ip, ok := netip.AddrFromSlice(cidr.Ip); ok {
					prefixes = append(prefixes, netip.PrefixFrom(ip, int(cidr.Prefix)))
				}
			}
			break
		}
	}
	return prefixes, nil
}

func setupNetwork() error {
	time.Sleep(3 * time.Second)

	ips, _ := net.LookupIP(vpsAddress)
	vpsIP, _ := netip.AddrFromSlice(ips[0].To4())
	physLUID, nextHop, _ := getPhysicalGateway()
	tunIf, _ := net.InterfaceByName(tunName)
	tunLUID, _ := winipcfg.LUIDFromIndex(uint32(tunIf.Index))

	// 1. Маршрут до VPS
	physLUID.AddRoute(netip.PrefixFrom(vpsIP, 32), nextHop, 0)

	// 2. Читаем geoip.dat и направляем RU-трафик мимо туннеля
	ruPrefixes, err := getRURoutes()
	if err == nil {
		for _, prefix := range ruPrefixes {
			physLUID.AddRoute(prefix, nextHop, 0)
		}
	}

	// 3. Заворачиваем остальной мир в TUN
	tunLUID.AddRoute(netip.MustParsePrefix("0.0.0.0/1"), netip.IPv4Unspecified(), 0)
	tunLUID.AddRoute(netip.MustParsePrefix("128.0.0.0/1"), netip.IPv4Unspecified(), 0)

	return nil
}

func teardownNetwork() error {
	ips, err := net.LookupIP(vpsAddress)
	if err == nil && len(ips) > 0 {
		vpsIP, _ := netip.AddrFromSlice(ips[0].To4())
		physLUID, nextHop, err := getPhysicalGateway()
		if err == nil {
			physLUID.DeleteRoute(netip.PrefixFrom(vpsIP, 32), nextHop)
			
			// Удаляем маршруты RU
			ruPrefixes, err := getRURoutes()
			if err == nil {
				for _, prefix := range ruPrefixes {
					physLUID.DeleteRoute(prefix, nextHop)
				}
			}
		}
	}

	tunIf, err := net.InterfaceByName(tunName)
	if err == nil {
		tunLUID, _ := winipcfg.LUIDFromIndex(uint32(tunIf.Index))
		tunLUID.DeleteRoute(netip.MustParsePrefix("0.0.0.0/1"), netip.IPv4Unspecified())
		tunLUID.DeleteRoute(netip.MustParsePrefix("128.0.0.0/1"), netip.IPv4Unspecified())
	}

	return nil
}