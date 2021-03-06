package controllers

import (
	"encoding/json"
	"fmt"
	"goTello/app/models"
	"goTello/config"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strconv"
)

var appContext struct {
	DroneManager *models.DroneManager
}

func init() {
	appContext.DroneManager = models.NewDroneManager()
}

// getTemplate connects base.html and other ~~.html file
func getTemplate(temp string) (*template.Template, error) {
	return template.ParseFiles("app/views/base.html", temp)
}

func viewIndexHandler(w http.ResponseWriter, req *http.Request) {
	t, err := getTemplate("app/views/index.html")
	
	if err != nil{
	    log.Println("Error when parsing a template: %s", err)
	}
	
	err = t.Execute(w, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func viewControllerHandler(w http.ResponseWriter, req *http.Request) {
	t, err := getTemplate("app/views/controller.html")
	
	if err != nil {
	    log.Println("Error when parsing a template: %s", err)
	}
	
	err = t.Execute(w, nil)
	if err != nil {
	    http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type APIResult struct {
	Result interface{} `json:"result"`
	Code   int         `json:"code"`
}

func APIResponse(w http.ResponseWriter, result interface{}, code int) {
	res := APIResult{Result: result, Code: code}
	js, err := json.Marshal(res)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(js)
}

// apiValidPath
// urlを正規表現でバリデーションしている。
var apiValidPath = regexp.MustCompile("^/api/(command|video)")

// apiMakeHandler 上記apiValidPathのurlと一致しているか、validationするラップ関数
func apiMakeHandler(fn func(w http.ResponseWriter, req *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// urlが正しいか検証した後、fn(w, req)返す
		m := apiValidPath.FindStringSubmatch(req.URL.Path)
		if len(m) == 0 {
		    APIResponse(w, "Not found", http.StatusNotFound)
		    return
		}
	
		// func(*Regexp) FindStringSubmatch の戻り値 nil は，マッチしないことを示します
		//if m == nil {
		//    APIResponse(w, "Not found", http.StatusNotFound)
		//    return
		//}
		
		
		fn(w, req)
	}
}

func getSpeed(req *http.Request) int {
	// req.ParseForm()
	// req.Form["speed"]
	
	strSpeed := req.FormValue("speed")
	if strSpeed == "" {
		return models.DefaultSpeed
	}
	speed, err := strconv.Atoi(strSpeed)
	if err != nil {
		return models.DefaultSpeed
	}
	return speed
}

func apiCommandHandler(w http.ResponseWriter, req *http.Request) {
	// req.ParseForm()
	// command := req.Form["command"]
	
	command := req.FormValue("command")
	log.Printf("action=apiCommandHandler command=%s", command)
	drone := appContext.DroneManager
	switch command {
	case "ceaseRotation":
		drone.CeaseRotation()
	case "takeOff":
		drone.TakeOff()
	case "land":
		drone.Land()
	case "hover":
		drone.Hover()
	case "up":
		drone.Up(drone.Speed)
	case "clockwise":
		drone.Clockwise(drone.Speed)
	case "counterClockwise":
		drone.CounterClockwise(drone.Speed)
	case "down":
		drone.Down(drone.Speed)
	case "forward":
		drone.Forward(drone.Speed)
	case "left":
		drone.Left(drone.Speed)
	case "right":
		drone.Right(drone.Speed)
	case "backward":
		drone.Backward(drone.Speed)
	case "speed":
		drone.Speed = getSpeed(r)
	case "frontFlip":
		drone.FrontFlip()
	case "leftFlip":
		drone.LeftFlip()
	case "rightFlip":
		drone.RightFlip()
	case "backFlip":
		drone.BackFlip()
	case "throwTakeOff":
		drone.ThrowTakeOff()
	case "bounce":
		drone.Bounce()
	case "patrol":
		drone.StartPatrol()
	case "stopPatrol":
		drone.StopPatrol()
	case "stopFaceDetectTrack":
		drone.DisableFaceDetectTrackingOn()
	case "faceDetectTrack":
		drone.EnableFaceDetectTracking()
	case "snapshot":
		drone.TakeSnapShot()
	default:
		APIResponse(w, "Not found", http.StatusNotFound)
		return
	}
	APIResponse(w, "OK", http.StatusOK)
}

func StartWebServer() error {
	http.HandleFunc("/", viewIndexHandler)
	http.HandleFunc("/controller/", viewControllerHandler)
	http.HandleFunc("/api/command/", apiMakeHandler(apiCommandHandler))
	
	// github.com/hybridgroup/mjpeg でfunc(s *Stream)ServeHTTP(w, req)実装してるから、
	// http.Handleの第二引数に入れれる。
	http.Handle("video/streaming", appContext.DroneManager.Stream)
	
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	return http.ListenAndServe(fmt.Sprintf("%s:%d", config.Config.Address, config.Config.Port), nil)
}
