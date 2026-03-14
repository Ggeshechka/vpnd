//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
	_ "github.com/xtls/xray-core/main/json"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

const (
	tunName = "xray-tun"
	vpsIP   = "188.40.167.82"
)

// Получаем IP физического адаптера через фиктивное UDP соединение
func getPhysicalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

// Получаем интерфейс и шлюз по умолчанию
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

// Запуск Xray с динамической инъекцией sendThrough
func startXray() (*core.Instance, error) {
	physIP, err := getPhysicalIP()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения физического IP: %v", err)
	}

	log.Printf("Обнаружен физический IP: %s", physIP)

	data, err := os.ReadFile("config.json")
	if err != nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Инъекция sendThrough для proxy и direct
	outbounds := config["outbounds"].([]interface{})
	for _, out := range outbounds {
		o := out.(map[string]interface{})
		tag := o["tag"].(string)
		if tag == "proxy" || tag == "direct" {
			o["sendThrough"] = physIP
		}
	}

	newConfigBytes, _ := json.Marshal(config)
	pbConfig, err := serial.DecodeJSONConfig(bytes.NewReader(newConfigBytes))
	if err != nil {
		return nil, err
	}

	cfg, err := pbConfig.Build()
	if err != nil {
		return nil, err
	}

	server, err := core.New(cfg)
	if err != nil {
		return nil, err
	}

	if err := server.Start(); err != nil {
		return nil, err
	}

	return server, nil
}

func setupWindowsRouting() error {
	time.Sleep(3 * time.Second) // Ждем инициализации TUN от Xray

	physLUID, nextHop, err := getPhysicalGateway()
	if err != nil {
		return fmt.Errorf("ошибка физического интерфейса: %v", err)
	}

	tunIf, err := net.InterfaceByName(tunName)
	if err != nil {
		return fmt.Errorf("интерфейс %s не найден (проверьте права Администратора): %v", tunName, err)
	}
	tunLUID, err := winipcfg.LUIDFromIndex(uint32(tunIf.Index))
	if err != nil {
		return err
	}

	// 1. Назначаем IP-адрес TUN-интерфейсу
	tunIP := netip.MustParsePrefix("172.19.0.2/24")
	_ = tunLUID.SetIPAddresses([]netip.Prefix{tunIP})

	// 2. Исключение для VPS: маршрут через физический шлюз
	serverIP := netip.MustParseAddr(vpsIP)
	err = physLUID.AddRoute(netip.PrefixFrom(serverIP, 32), nextHop, 0)
	if err != nil {
		log.Printf("ВНИМАНИЕ: Ошибка маршрута до VPS (петля возможна): %v", err)
	}

	// 3. Заворачиваем весь трафик системы в TUN
	_ = tunLUID.AddRoute(netip.MustParsePrefix("0.0.0.0/1"), netip.IPv4Unspecified(), 0)
	_ = tunLUID.AddRoute(netip.MustParsePrefix("128.0.0.0/1"), netip.IPv4Unspecified(), 0)

	log.Println("Таблица маршрутизации обновлена успешно.")
	return nil
}

func main() {
	os.Setenv("xray.location.asset", ".")

	log.Println("Запуск Xray-core...")
	xrayServer, err := startXray()
	if err != nil {
		log.Fatalf("Ошибка запуска Xray: %v", err)
	}
	defer xrayServer.Close()

	log.Println("Настройка маршрутизации Windows...")
	if err := setupWindowsRouting(); err != nil {
		log.Fatalf("Ошибка маршрутизации: %v", err)
	}

	log.Println("VPN успешно работает. Нажмите Ctrl+C для выхода.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Остановка...")
}
