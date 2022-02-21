package config

import (
	"gopkg.in/ini.v1"
	"log"
	"os"
)

type ConfList struct {
	LogFile string
	Address string
	Port    int
}

var Config ConfList

func init() {
	// config.iniファイルから設定情報読み込み
	cfg, err := ini.Load("config.ini")
	if err != nil {
		log.Printf("Failed to read file: %v", err)
		os.Exit(1)
	}

	// Config構造体に値を設定（main.goで使用）
	Config = ConfList{
		LogFile: cfg.Section("gotello").Key("log_file").String(),
		Address: cfg.Section("web").Key("address").String(),
		Port:    cfg.Section("web").Key("port").MustInt(),
	}
}
