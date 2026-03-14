//go:build windows
package main

import (
	"fmt"
	"net"
	"net/netip"
	"time"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

const (
	tunName    = "xray0"
	vpsAddress = "de.safelane.pro" // Ваш сервер
)

// getPhysicalGateway читает таблицу маршрутизации и находит адаптер с интернетом
func getPhysicalGateway() (winipcfg.LUID, netip.Addr, error) {
	routes, err := winipcfg.GetIPForwardTable2(windows.AF_INET)
	if err != nil {
		return 0, netip.Addr{}, err
	}

	var bestLUID winipcfg.LUID
	var bestNextHop netip.Addr
	var lowestMetric uint32 = ^uint32(0) // Максимальное значение

	for _, r := range routes {
		// Ищем дефолтный маршрут ОС (0.0.0.0/0)
		if r.DestinationPrefix.PrefixLength == 0 {
			if r.Metric < lowestMetric {
				lowestMetric = r.Metric
				bestLUID = r.InterfaceLUID
				bestNextHop = r.NextHop.Addr()
			}
		}
	}

	if lowestMetric == ^uint32(0) {
		return 0, netip.Addr{}, fmt.Errorf("маршрут по умолчанию не найден")
	}

	return bestLUID, bestNextHop, nil
}

func setupNetwork() error {
	time.Sleep(3 * time.Second) // Ждем инициализации TUN интерфейса ядром Xray

	ips, err := net.LookupIP(vpsAddress)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("ошибка разрешения DNS сервера: %v", err)
	}
	vpsIP, ok := netip.AddrFromSlice(ips[0].To4())
	if !ok {
		return fmt.Errorf("получен невалидный IPv4 адрес для сервера")
	}

	physLUID, nextHop, err := getPhysicalGateway()
	if err != nil {
		return fmt.Errorf("шлюз не найден: %v", err)
	}

	tunIf, err := net.InterfaceByName(tunName)
	if err != nil {
		return fmt.Errorf("интерфейс %s не найден: %v", tunName, err)
	}
	tunLUID, err := winipcfg.LUIDFromIndex(uint32(tunIf.Index))
	if err != nil {
		return fmt.Errorf("ошибка получения LUID: %v", err)
	}

	// 1. Исключение: пускаем трафик до сервера мимо VPN (через физический шлюз)
	err = physLUID.AddRoute(netip.PrefixFrom(vpsIP, 32), nextHop, 0)
	if err != nil {
		return fmt.Errorf("ошибка добавления маршрута VPS: %v", err)
	}

	// 2. Захват трафика: 0.0.0.0/1 и 128.0.0.0/1 (работает безотказно в Windows)
	err = tunLUID.AddRoute(netip.MustParsePrefix("0.0.0.0/1"), netip.IPv4Unspecified(), 0)
	if err != nil {
		return fmt.Errorf("ошибка туннелирования (0.0.0.0/1): %v", err)
	}
	err = tunLUID.AddRoute(netip.MustParsePrefix("128.0.0.0/1"), netip.IPv4Unspecified(), 0)
	if err != nil {
		return fmt.Errorf("ошибка туннелирования (128.0.0.0/1): %v", err)
	}

	return nil
}

func teardownNetwork() error {
	ips, err := net.LookupIP(vpsAddress)
	if err == nil && len(ips) > 0 {
		vpsIP, ok := netip.AddrFromSlice(ips[0].To4())
		if ok {
			physLUID, nextHop, err := getPhysicalGateway()
			if err == nil {
				physLUID.DeleteRoute(netip.PrefixFrom(vpsIP, 32), nextHop)
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