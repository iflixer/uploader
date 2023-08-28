package queue

import (
	"log"
	"time"
	"uploader/db"
	"uploader/ffmpeg"
	"uploader/s3serv"
	"uploader/torrentServ"
)

// this service should watch the new tasks in DB and process them

type Queue struct {
	db *db.Db
	s3 *s3serv.S3serv
}

func (f *Queue) RunAsync() error {
	go f.worker()
	return nil
}

// Add saves the task to DB
func (f *Queue) Add(ff *ffmpeg.Ffmpeg) error {
	// create the task in DB
	return f.db.AddTask(ff)
}

func (f *Queue) worker() {
	for {
		log.Println("tick")
		if ff, err := f.db.GetTask(); err == nil {
			switch ff.Name {
			case "convert":
				if err := ff.Convert(); err == nil {
					f.db.FinishTask(ff, "done")
				} else {
					f.db.FinishTask(ff, "fail")
				}
			case "torrent":
				filename, ok := torrentServ.Download(ff.Magnet)
				if ok {
					f.db.FinishTask(ff, "done")
					ff.FileName = "/files/" + filename
					ff.Name = "convert"
					f.Add(ff)
				} else {
					f.db.FinishTask(ff, "fail")
				}
			default:
				f.db.FinishTask(ff, "wrongname")
				log.Println("task type not found:", ff.Name)
			}
		} else {
			log.Println("get task error:", err)
		}
		time.Sleep(time.Second * 10)
	}
}

func NewQueue(srvDb *db.Db, srv3serv *s3serv.S3serv) (*Queue, error) {
	return &Queue{
		db: srvDb,
		s3: srv3serv,
	}, nil
}
