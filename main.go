package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all"
	_ "github.com/xtls/xray-core/main/json"
)

const (
	tunName = "xray-tun"
	vpsIP   = "188.40.167.82"
)

func getPhysicalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

func startXray() (*core.Instance, error) {
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
		return nil, err
	}

	outbounds := config["outbounds"].([]interface{})
	for _, out := range outbounds {
		o := out.(map[string]interface{})
		tag := o["tag"].(string)
		if tag == "proxy" || tag == "direct" {
			configureOutbound(o, physIP)
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

func main() {
	os.Setenv("xray.location.asset", ".")

	xrayServer, err := startXray()
	if err != nil {
		log.Fatalf("Ошибка Xray: %v", err)
	}
	defer xrayServer.Close()

	if err := setupRouting(); err != nil {
		log.Fatalf("Ошибка маршрутизации: %v", err)
	}

	log.Println("VPN работает. Нажмите Ctrl+C для выхода.")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}