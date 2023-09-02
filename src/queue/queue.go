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

func (q *Queue) RunAsync() error {
	go q.worker()
	return nil
}

// Add saves the task to DB
func (q *Queue) Add(ff *ffmpeg.Ffmpeg) error {
	// create the task in DB
	return q.db.AddTask(ff)
}

func (q *Queue) Tasks() ([]db.TasksTable, error) {
	return q.db.TaskList()
}

func (q *Queue) worker() {
	for {
		// log.Println("tick")
		if ff, err := q.db.GetTask(); err == nil {
			switch ff.Name {
			case "convert":
				q.db.UpdateTask(ff, "working")
				if err := ff.Convert(); err == nil {
					q.db.UpdateTask(ff, "done")
					ff.Name = "upload"
					q.Add(ff)
				} else {
					q.db.FinishTask(ff, "fail")
				}
			case "upload":
				q.db.UpdateTask(ff, "working")
				if err := q.s3.Add(ff); err == nil {
					q.db.FinishTask(ff, "done")
				} else {
					q.db.FinishTask(ff, "fail")
				}
			case "torrent":
				q.db.UpdateTask(ff, "working")
				// TODO: spawn a goroutine here
				filename, ok := torrentServ.Download(ff.Magnet)
				if ok {
					q.db.FinishTask(ff, "done")
					ff.FileName = filename
					ff.Name = "convert"
					q.Add(ff)
				} else {
					q.db.FinishTask(ff, "fail")
				}
			default:
				q.db.FinishTask(ff, "wrongname")
				log.Println("task type not found:", ff.Name)
			}
		} else {
			// log.Println("get task error:", err)
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
