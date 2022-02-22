package models

import (
	"context"
	"github.com/hybridgroup/mjpeg"
	"gobot.io/x/gobot"
	"gobot.io/x/gobot/platforms/dji/tello"
	"gocv.io/x/gocv"
	"golang.org/x/sync/semaphore"
	"image"
	"image/color"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os/exec"
	"strconv"
	"time"
)

const (
	DefaultSpeed      = 10
	WaitDroneStartSec = 5

	frameX       = 960 / 3
	frameY       = 720 / 3
	frameCenterX = frameX / 2
	frameCenterY = frameY / 2
	frameArea    = frameX * frameY
	frameSize    = frameArea * 3

	// 人間の顔を識別する為のopenCVのxmlファイルダウンロード
	faceDetectXMLFile = "./app/models/haarcascade_frontalface_default.xml"
	// ドローンから撮ったスナップショット写真格納フォルダ
	snapShotsFolder = "./static/img/snapshots/"
)

// DroneManager　既存のtello.Driver構造体に
// 新たなフィールド追加するため
type DroneManager struct {
	*tello.Driver

	Speed                int
	patrolSem            *semaphore.Weighted
	patrolQuit           chan bool
	isPatrolling         bool
	ffmpegIn             io.WriteCloser
	ffmpegOut            io.ReadCloser
	Stream               *mjpeg.Stream
	faceDetectTrackingOn bool
	isSnapShot           bool
}

// NewDroneManager 以下参考url
// https://gobot.io/documentation/examples/tello_video/
func NewDroneManager() *DroneManager {
	drone := tello.NewDriver("8889")

	ffmpeg := exec.Command("ffmpeg", "-hwaccel", "auto", "-hwaccel_device", "opencl", "-i", "pipe:0", "-pix_fmt", "bgr24",
		"-s", strconv.Itoa(frameX)+"x"+strconv.Itoa(frameY), "-f", "rawvideo", "pipe:1")
	ffmpegIn, _ := ffmpeg.StdinPipe()
	ffmpegOut, _ := ffmpeg.StdoutPipe()

	droneManager := &DroneManager{
		Driver: drone,

		Speed:                DefaultSpeed,
		patrolSem:            semaphore.NewWeighted(1),
		patrolQuit:           make(chan bool),
		isPatrolling:         false,
		ffmpegIn:             ffmpegIn,
		ffmpegOut:            ffmpegOut,
		Stream:               mjpeg.NewStream(),
		faceDetectTrackingOn: false,
		isSnapShot:           false,
	}

	work := func() {
		// コネクトされた時のイベント追加
		//
		// Event handler
		// func(tello.NewDriver)On(name string, f func(s interface{})) (err error)

		drone.On(tello.ConnectedEvent, func(data interface{}) {
			log.Println("Connected to your Drone")
			err := drone.StartVideo()
			if err != nil {
				log.Println("ERROR: drone.StartVideo()", err)
				return
			}

			err = drone.SetVideoEncoderRate(tello.VideoBitRateAuto)
			if err != nil {
				log.Println("ERROR: drone.SetVideoEncoderRate(tello.VideoBitRateAuto)", err)
				return
			}

			// ドローンのカメラの画質設定
			err = drone.SetExposure(0)
			if err != nil {
				log.Println("ERROR: drone.SetExposure(0)", err)
				return
			}

			gobot.Every(100*time.Millisecond, func() {
				err = drone.StartVideo()
				if err != nil {
					log.Println("ERROR: drone.StartVideo()", err)
					return
				}
			})

			droneManager.StreamVideo()
		})

		// VideoFrameにイベントをセット
		drone.On(tello.VideoFrameEvent, func(data interface{}) {
			pkt := data.([]byte)
			log.Println(pkt)
		})
	}
	robot := gobot.NewRobot("tello", []gobot.Connection{}, []gobot.Device{drone}, work)
	go func() {
		err := robot.Start()
		if err != nil {
			log.Println("ERROR: robot.Start()", err)
			return
		}
	}()

	time.Sleep(WaitDroneStartSec * time.Second)
	return droneManager
}

// Patrol はgoroutineで走っているから、Patrolが実行中にまたよばれると、semaphoreとれない.
// よって既にPatrol()呼ばれている場合はロック取れないので
// isAcquire == false 入る。 その処理の中でパトロール中止命令
// 呼ばれていなかった場合、Patrol()開始。
func (d *DroneManager) Patrol() {
	go func() {

		// 同時にこの関数を走らせる数を
		// セマフォで１つに縛る
		//
		// TryAcquire()はロックがとれなくてもブロッキングしない
		isAcquire := d.patrolSem.TryAcquire(1)
		if !isAcquire {
			d.patrolQuit <- true
			d.isPatrolling = false
			return
		}

		// for抜けたら、確実にセマフォリリース
		defer d.patrolSem.Release(1)
		d.isPatrolling = true

		// status状態によってドローンの動き変えていく
		status := 0

		// 3秒毎にドローンの動きを変えていこうか。。
		t := time.NewTicker(3 * time.Second)
		for {
			select {
			case <-t.C:
				d.Hover()
				switch status {
				case 1:
					d.Forward(d.Speed)
				case 2:
					d.Right(d.Speed)
				case 3:
					d.Backward(d.Speed)
				case 4:
					d.Left(d.Speed)
				case 5:
					status = 0
				}
				status++

			case <-d.patrolQuit:
				//既にPatrol()呼ばれている時に、再度Patrol()
				//呼ばれたらここのcase入る

				// tickerタイマーをストップ
				t.Stop()
				// そしてドローンを一時停止
				d.Hover()
				d.isPatrolling = false
				return
			}
		}
	}()
}

func (d *DroneManager) StartPatrol() {
	if !d.isPatrolling {
		d.Patrol()
	}
}

func (d *DroneManager) StopPatrol() {
	if d.isPatrolling {
		d.Patrol()
	}
}

func (d *DroneManager) StreamVideo() {
	go func(d *DroneManager) {
		classifier := gocv.NewCascadeClassifier()
		defer func() {
			err := classifier.Close()
			if err != nil {
				log.Println("ERROR: classifier.Close()", err)
				return
			}
		}()

		if !classifier.Load(faceDetectXMLFile) {
			log.Println("ERROR: classifier.Load(faceDetectXMLFile)")
			return
		}

		// 青色のフレームにしたい
		blue := color.RGBA{0, 0, 255, 0}

		for {
			buf := make([]byte, frameSize)
			_, err := io.ReadFull(d.ffmpegOut, buf)
			if err != nil {
				log.Println(err)
			}

			img, _ := gocv.NewMatFromBytes(frameY, frameX, gocv.MatTypeCV8UC3, buf)

			if img.Empty() {
				continue
			}

			if d.faceDetectTrackingOn {
				d.StopPatrol()

				// 人の顔をレクタングル（長方形）で表示
				rects := classifier.DetectMultiScale(img)
				// 長方形の数イコール人の顔の数ってことや
				log.Printf("found %d faces\n", len(rects))

				if len(rects) == 0 {
					d.Hover()
				}

				for _, r := range rects {
					gocv.Rectangle(&img, r, blue, 3)
					pt := image.Pt(r.Max.X, r.Min.Y-5)
					gocv.PutText(&img, "人間ココイル", pt, gocv.FontHersheyPlain, 1.2, blue, 2)

					faceWidth := r.Max.X - r.Min.X
					faceHeight := r.Max.Y - r.Min.Y
					faceCenterX := r.Min.X + (faceWidth / 2)
					faceCenterY := r.Min.Y + (faceHeight / 2)
					faceArea := faceWidth * faceHeight
					diffX := frameCenterX - faceCenterX
					diffY := frameCenterY - faceCenterY
					percentF := math.Round(float64(faceArea) / float64(frameArea) * 100)

					// カメラのフレーム内にドローンを移動
					move := false

					if diffX < -20 {
						d.Right(15)
						move = true
					}

					if diffX > 20 {
						d.Left(15)
						move = true
					}

					if diffY < -30 {
						d.Down(25)
						move = true
					}

					if diffY > 30 {
						d.Up(25)
						move = true
					}

					if percentF > 7.0 {
						d.Backward(10)
						move = true
					}

					if percentF < 0.9 {
						d.Forward(10)
						move = true
					}

					if !move {
						d.Hover()
					}

					break
				}

				jpegBuf, _ := gocv.IMEncode(".jpg", img)

				// TakeSnapShotメソッドが呼ばれると、
				// その中で、d.isSnapShot == true になる。
				//
				// TakeSnapShot関数が呼ばれた時のjpegBufを
				// 作成したフォルダに書き込む
				if d.isSnapShot {
					backupFileName := snapShotsFolder + time.Now().Format(time.RFC3339) + ".jpg"
					_ = ioutil.WriteFile(backupFileName, jpegBuf, 0644)

					snapShotFileName := snapShotsFolder + "snapshot.jpg"
					_ = ioutil.WriteFile(snapShotFileName, jpegBuf, 0644)

					d.isSnapShot = false
				}

				d.Stream.UpdateJPEG(jpegBuf)
			}
		}
	}(d)
}

func (d *DroneManager) TakeSnapShot() {
	d.isSnapShot = true
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		if !d.isSnapShot || ctx.Err() != nil {
			break
		}
	}
	//d.isSnapShot = false
}

func (d *DroneManager) EnableFaceDetectTracking() {
	d.faceDetectTrackingOn = true
}

func (d *DroneManager) DisableFaceDetectTrackingOn() {
	d.faceDetectTrackingOn = false
	d.Hover()
}
