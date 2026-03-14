//go:build windows
package main

import (
	"fmt"
	"net"
	"net/netip"
	"time"

	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

const (
	tunName    = "xray0"
	vpsAddress = "de.safelane.pro" // Ваш сервер
)

func setupNetwork() error {
	time.Sleep(3 * time.Second) // Ждем инициализации TUN

	// 1. Узнаем IP сервера
	ips, err := net.LookupIP(vpsAddress)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("ошибка DNS сервера: %v", err)
	}
	vpsIP, _ := netip.AddrFromSlice(ips[0].To4())

	// 2. Находим текущий маршрут до сервера (физический шлюз)
	bestRoute, err := winipcfg.GetBestRoute(vpsIP)
	if err != nil {
		return fmt.Errorf("шлюз не найден: %v", err)
	}

	// 3. Находим TUN интерфейс
	tunIf, err := net.InterfaceByName(tunName)
	if err != nil {
		return fmt.Errorf("интерфейс %s не найден: %v", tunName, err)
	}
	tunLUID, _ := winipcfg.LUIDFromIndex(uint32(tunIf.Index))

	// 4. Исключение: пускаем IP сервера мимо VPN через физический адаптер
	err = bestRoute.InterfaceLUID.AddRoute(vpsIP.Prefix(32), bestRoute.NextHop, 0)
	if err != nil {
		return fmt.Errorf("ошибка маршрута VPS: %v", err)
	}

	// 5. Заворачиваем весь остальной трафик Windows в TUN
	err = tunLUID.AddRoute(netip.MustParsePrefix("0.0.0.0/0"), netip.IPv4Unspecified(), 5)
	if err != nil {
		return fmt.Errorf("ошибка туннелирования: %v", err)
	}

	return nil
}

func teardownNetwork() error {
	ips, err := net.LookupIP(vpsAddress)
	if err == nil && len(ips) > 0 {
		vpsIP, _ := netip.AddrFromSlice(ips[0].To4())
		bestRoute, err := winipcfg.GetBestRoute(vpsIP)
		if err == nil {
			bestRoute.InterfaceLUID.DeleteRoute(vpsIP.Prefix(32), bestRoute.NextHop)
		}
	}

	tunIf, err := net.InterfaceByName(tunName)
	if err == nil {
		tunLUID, _ := winipcfg.LUIDFromIndex(uint32(tunIf.Index))
		tunLUID.DeleteRoute(netip.MustParsePrefix("0.0.0.0/0"), netip.IPv4Unspecified())
	}

	return nil
}