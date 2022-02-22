package main

import (
	"goTello/app/controllers"
	"goTello/config"
	"goTello/utils"
	"log"
)

func main() {
	utils.LoggingSettings(config.Config.LogFile)
	log.Println(controllers.StartWebServer())
}
