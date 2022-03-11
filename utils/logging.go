package utils

import (
	"io"
	"log"
	"os"
)

// LoggingSettings is for saving log in specific logFile and Terminal.
func LoggingSettings(logFile string) {
	logfile, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("file=logFile err=%s", err.Error())
	}
	multiLogFile := io.MultiWriter(os.Stdout, logfile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	
	// 標準ロガーの出力先をmultiLogFilwに設定。
	log.SetOutput(multiLogFile)
}
