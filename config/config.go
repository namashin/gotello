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

// Config 別パッケージからでも使えるように、頭文字を大文字に。
var Config ConfList

// init
// main.go実行される前に設定ファイル読み込む
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
