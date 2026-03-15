//go:build windows

package main

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"time"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

func configureOutbound(o map[string]interface{}, physIP string) {
	o["sendThrough"] = physIP
}

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

func setupRouting() error {
	time.Sleep(3 * time.Second)

	physLUID, nextHop, err := getPhysicalGateway()
	if err != nil {
		return err
	}

	tunIf, err := net.InterfaceByName(tunName)
	if err != nil {
		return err
	}
	tunLUID, err := winipcfg.LUIDFromIndex(uint32(tunIf.Index))
	if err != nil {
		return err
	}

	tunIP := netip.MustParsePrefix("172.19.0.2/24")
	_ = tunLUID.SetIPAddresses([]netip.Prefix{tunIP})

	serverIP := netip.MustParseAddr(vpsIP)
	err = physLUID.AddRoute(netip.PrefixFrom(serverIP, 32), nextHop, 0)
	if err != nil {
		log.Printf("ВНИМАНИЕ: Ошибка маршрута до VPS: %v", err)
	}

	_ = tunLUID.AddRoute(netip.MustParsePrefix("0.0.0.0/1"), netip.IPv4Unspecified(), 0)
	_ = tunLUID.AddRoute(netip.MustParsePrefix("128.0.0.0/1"), netip.IPv4Unspecified(), 0)

	return nil
}