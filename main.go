package main

import (
	"bytes"
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/sagernet/sing-box/box"
	"github.com/sagernet/sing-box/option"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
	_ "github.com/xtls/xray-core/main/json"
)

func main() {
	// Указываем Xray, где искать geoip.dat и geosite.dat
	os.Setenv("xray.location.asset", ".")

	log.Println("Инициализация Xray (SOCKS5)...")
	xrayBytes, err := os.ReadFile("config.json")
	if err != nil {
		log.Fatalf("Ошибка чтения config.json: %v", err)
	}

	pbConfig, err := serial.DecodeJSONConfig(bytes.NewReader(xrayBytes))
	if err != nil {
		log.Fatalf("Ошибка парсинга config.json: %v", err)
	}

	xrayConf, _ := pbConfig.Build()
	xrayServer, err := core.New(xrayConf)
	if err != nil {
		log.Fatalf("Ошибка создания Xray: %v", err)
	}

	if err := xrayServer.Start(); err != nil {
		log.Fatalf("Ошибка запуска Xray: %v", err)
	}
	defer xrayServer.Close()

	log.Println("Инициализация Sing-box (WFP TUN)...")
	tunBytes, err := os.ReadFile("tun.json")
	if err != nil {
		log.Fatalf("Ошибка чтения tun.json: %v", err)
	}

	tunOptions, err := option.Parse(tunBytes)
	if err != nil {
		log.Fatalf("Ошибка парсинга tun.json: %v", err)
	}

	singBox, err := box.New(box.Options{
		Context: context.Background(),
		Options: tunOptions,
	})
	if err != nil {
		log.Fatalf("Ошибка создания Sing-box: %v", err)
	}

	if err := singBox.Start(); err != nil {
		log.Fatalf("Ошибка запуска Sing-box: %v", err)
	}
	defer singBox.Close()

	log.Println("VPN успешно запущен. Нажмите Ctrl+C для остановки.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Остановка сервисов...")
}
