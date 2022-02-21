package models

import (
	"gobot.io/x/gobot"
	"gobot.io/x/gobot/platforms/dji/tello"
	"golang.org/x/sync/semaphore"
	"log"
	"time"
)

const (
	DefaultSpeed      = 10
	WaitDroneStartSec = 5
)

// DroneManager　既存のtello.Driver構造体に
// 新たなフィールド追加
type DroneManager struct {
	*tello.Driver
	
	Speed        int
	patrolSem    *semaphore.Weighted
	patrolQuit   chan bool
	isPatrolling bool
}

func NewDroneManager() *DroneManager {
	drone := tello.NewDriver("8889")
	droneManager := &DroneManager{
		Driver:       drone,

		Speed:        DefaultSpeed,
		patrolSem:    semaphore.NewWeighted(1),
		patrolQuit:   make(chan bool),
		isPatrolling: false,
	}
	work := func() {
		// コネクトされた時のイベント追加
		drone.On(tello.ConnectedEvent, func(data interface{}) {
			log.Println("Connected")
			err := drone.StartVideo()
			if err != nil{
				log.Println("ERROR: drone.StartVideo()", err)
				return
			}

			err = drone.SetVideoEncoderRate(tello.VideoBitRateAuto)
			if err != nil{
				log.Println("ERROR: drone.SetVideoEncoderRate(tello.VideoBitRateAuto)", err)
				return
			}

			err = drone.SetExposure(0)
			if err != nil{
				log.Println("ERROR: drone.SetExposure(0)", err)
				return
			}

			gobot.Every(100*time.Millisecond, func() {
				err = drone.StartVideo()
				if err != nil{
					log.Println("ERROR: drone.StartVideo()", err)
					return
				}
			})
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
		if err != nil{
			log.Println("ERROR: robot.Start()", err)
			return
		}
	}()

	time.Sleep(WaitDroneStartSec * time.Second)
	return droneManager
}

// Patrol はgoroutineで走っているから、Patrolが実行中にまたよばれると、semaphoreとれない.
// よって既にPatrol()呼ばれている場合はロック取れないので
// isAcquire == false 入る。
// 呼ばれていなかった場合、Patrol()開始。
func (d *DroneManager) Patrol() {
	go func() {
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
		t := time.NewTicker(3 * time.Second)
		for {
			select {
			// 3秒ごとに入ってくる
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
				//呼ばれたらここcase入る

				// タイマーをストップ
				t.Stop()
				// そして一時停止
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
