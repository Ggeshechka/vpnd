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
	vpsAddress = "188.40.167.82" // Жестко заданный IP сервера во избежание петель DNS
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
	time.Sleep(3 * time.Second) // Ждем инициализации TUN адаптера

	vpsIP, err := netip.ParseAddr(vpsAddress)
	if err != nil {
		return fmt.Errorf("ошибка парсинга IP сервера: %v", err)
	}

	physLUID, nextHop, err := getPhysicalGateway()
	if err != nil {
		return fmt.Errorf("ошибка получения шлюза: %v", err)
	}

	tunIf, err := net.InterfaceByName(tunName)
	if err != nil {
		return fmt.Errorf("интерфейс %s не найден: %v", tunName, err)
	}
	tunLUID, err := winipcfg.LUIDFromIndex(uint32(tunIf.Index))
	if err != nil {
		return fmt.Errorf("ошибка LUID TUN: %v", err)
	}

	// 1. Исключение: маршрут до VPS через физический шлюз
	err = physLUID.AddRoute(netip.PrefixFrom(vpsIP, 32), nextHop, 0)
	if err != nil {
		fmt.Printf("КРИТИЧЕСКАЯ ОШИБКА маршрута VPS (нужны права Администратора?): %v\n", err)
		return err
	}

	// 2. Исключение: RU-сети мимо VPN
	ruPrefixes, err := getRURoutes()
	if err == nil {
		for _, prefix := range ruPrefixes {
			_ = physLUID.AddRoute(prefix, nextHop, 0)
		}
	} else {
		fmt.Printf("Ошибка чтения geoip.dat: %v\n", err)
	}

	// 3. Заворачиваем весь остальной мир в TUN
	err = tunLUID.AddRoute(netip.MustParsePrefix("0.0.0.0/1"), netip.IPv4Unspecified(), 0)
	if err != nil {
		fmt.Printf("Ошибка маршрута 0.0.0.0/1: %v\n", err)
	}
	err = tunLUID.AddRoute(netip.MustParsePrefix("128.0.0.0/1"), netip.IPv4Unspecified(), 0)
	if err != nil {
		fmt.Printf("Ошибка маршрута 128.0.0.0/1: %v\n", err)
	}

	return nil
}

func teardownNetwork() error {
	vpsIP, err := netip.ParseAddr(vpsAddress)
	if err == nil {
		physLUID, nextHop, err := getPhysicalGateway()
		if err == nil {
			physLUID.DeleteRoute(netip.PrefixFrom(vpsIP, 32), nextHop)
			
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
		tunLUID, err := winipcfg.LUIDFromIndex(uint32(tunIf.Index))
		if err == nil {
			tunLUID.DeleteRoute(netip.MustParsePrefix("0.0.0.0/1"), netip.IPv4Unspecified())
			tunLUID.DeleteRoute(netip.MustParsePrefix("128.0.0.0/1"), netip.IPv4Unspecified())
		}
	}

	return nil
}
