//go:build windows
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"time"

	"github.com/xjasonlyu/tun2socks/v2/engine"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
	_ "github.com/xtls/xray-core/main/json"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

const (
	tunName   = "xray-tun"
	socksAddr = "socks5://127.0.0.1:10808" // Должен совпадать с портом в config.json
)

func getPhysicalIP() (string, error) {
	// Подключение не устанавливается физически (это UDP), 
	// но ОС определяет, через какой интерфейс пойдет пакет, и возвращает его IP.
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", fmt.Errorf("ошибка определения физического IP: %v", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// Запуск Xray с динамической подстановкой sendThrough
func startXrayDynamic() (*core.Instance, error) {
	physIP, err := getPhysicalIP()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения физического IP: %v", err)
	}

	data, err := os.ReadFile("config.json")
	if err != nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("ошибка парсинга json: %v", err)
	}

	// Защита от петли: Xray должен выходить в сеть только через физическую карту
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

func setupNetwork() error {
	// 1. Запуск ядра Xray
	_, err := startXrayDynamic()
	if err != nil {
		return fmt.Errorf("сбой запуска xray: %v", err)
	}

	// 2. Инициализация и запуск tun2socks
	key := &engine.Key{
		Device:   "tun://" + tunName,
		Proxy:    socksAddr,
		LogLevel: "warning",
	}
	engine.Insert(key)
	go engine.Start() // Запускаем в фоне, так как engine.Start() блокирует поток

	time.Sleep(3 * time.Second) // Даем время на создание TUN адаптера ОС Windows

	// 3. Перехват трафика: заворачиваем Windows в созданный TUN
	tunIf, err := net.InterfaceByName(tunName)
	if err != nil {
		return fmt.Errorf("интерфейс tun2socks не найден: %v", err)
	}
	tunLUID, _ := winipcfg.LUIDFromIndex(uint32(tunIf.Index))

	// Правила перехвата всего интернета
	_ = tunLUID.AddRoute(netip.MustParsePrefix("0.0.0.0/1"), netip.IPv4Unspecified(), 0)
	_ = tunLUID.AddRoute(netip.MustParsePrefix("128.0.0.0/1"), netip.IPv4Unspecified(), 0)

	return nil
}

func teardownNetwork() error {
	// 1. Возвращаем маршруты Windows в норму
	tunIf, err := net.InterfaceByName(tunName)
	if err == nil {
		tunLUID, _ := winipcfg.LUIDFromIndex(uint32(tunIf.Index))
		_ = tunLUID.DeleteRoute(netip.MustParsePrefix("0.0.0.0/1"), netip.IPv4Unspecified())
		_ = tunLUID.DeleteRoute(netip.MustParsePrefix("128.0.0.0/1"), netip.IPv4Unspecified())
	}
	
	// 2. Останавливаем tun2socks
	engine.Stop()
	return nil
}