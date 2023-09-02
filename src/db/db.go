package db

import (
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"regexp"
	"strings"
	"time"
	"uploader/ffmpeg"
)

// this service should watch the new tasks in DB and process them

type Db struct {
	serviceID int
	db        *sql.DB
}

type TasksTable struct {
	ID             int
	Name           string
	Filename       string
	ResultFilename string
	CreatedAt      time.Time
	Status         string
	ServiceID      int
	UserID         string
	OrigFileName   string
}

func (db *Db) Lock(taskID int) error {
	q := fmt.Sprintf(`INSERT INTO locks (serviceID, taskID) VALUES (%d, %d)`, db.serviceID, taskID)
	log.Println(q)
	res, err := db.db.Query(q)
	defer res.Close()
	if err != nil {
		fmt.Println(err)
		return err
	}

	// check is we really locked (compare serviceID)
	q = fmt.Sprintf(`SELECT serviceID FROM locks WHERE taskID=%d`, taskID)
	results, err := db.db.Query(q)
	defer results.Close()
	if err != nil {
		panic(err.Error())
	}
	if results.Next() {
		var sID int
		err = results.Scan(
			&sID,
		)
		if err != nil {
			return err
		}
		if sID != db.serviceID {
			return errors.New("taken by another thread")
		}
	}
	return nil
}

func (db *Db) UnLock(taskID int) error {
	q := fmt.Sprintf(`DELETE FROM locks WHERE taskID=%d`, taskID)
	log.Println(q)
	res, err := db.db.Query(q)
	if err != nil {
		fmt.Println(err)
		return err
	}
	res.Close()
	return nil
}

func (db *Db) AddTask(ff *ffmpeg.Ffmpeg) error {
	q := fmt.Sprintf("INSERT INTO `tasks` (`name`, `filename`, `status`, `serviceID`, `userID`, `origFileName`, `ResultFilename`) VALUES ('%s', '%s', '%s', %d, '%s', '%s', '%s')",
		ff.Name, ff.FileName, "created", db.serviceID, "", ff.OrigFileName, ff.FileNameResult)
	log.Println(q)
	res, err := db.db.Query(q)
	if err != nil {
		return err
	}
	res.Close()
	return nil
}

func (db *Db) GetTask() (*ffmpeg.Ffmpeg, error) {
	// get 1 task
	results, err := db.db.Query(`SELECT t.* FROM tasks t LEFT JOIN locks l ON (l.taskID = t.id)
                            WHERE t.status='created' AND ISNULL(l.id)
                            ORDER BY t.createdAt limit 1`)
	if err != nil {
		panic(err.Error())
	}
	if results.Next() {
		var tasksTable TasksTable
		err = results.Scan(
			&tasksTable.ID,
			&tasksTable.Name,
			&tasksTable.Filename,
			&tasksTable.CreatedAt,
			&tasksTable.Status,
			&tasksTable.ServiceID,
			&tasksTable.UserID,
			&tasksTable.OrigFileName,
			&tasksTable.ResultFilename,
		)
		if err != nil {
			panic(err.Error())
		}
		fmt.Println(tasksTable)

		if err := db.Lock(tasksTable.ID); err == nil {
			ff := ffmpeg.NewFfmpeg(tasksTable.OrigFileName, tasksTable.Filename, tasksTable.Name, tasksTable.ID)
			return ff, nil
		}
	}

	return nil, errors.New("no tasks")
}

func (db *Db) TaskList() (res []TasksTable, err error) {
	// get tasks list
	results, err := db.db.Query(`SELECT * FROM tasks order by id desc limit 50`)
	if err != nil {
		return nil, err
	}
	for results.Next() {
		var tasksTable TasksTable
		err = results.Scan(
			&tasksTable.ID,
			&tasksTable.Name,
			&tasksTable.Filename,
			&tasksTable.CreatedAt,
			&tasksTable.Status,
			&tasksTable.ServiceID,
			&tasksTable.UserID,
			&tasksTable.OrigFileName,
			&tasksTable.ResultFilename,
		)
		if err != nil {
			panic(err.Error())
		}
		res = append(res, tasksTable)
	}

	return res, nil
}

// delete the task
func (db *Db) RemoveTask(ff *ffmpeg.Ffmpeg) error {

	return nil
}

func (db *Db) UpdateTask(ff *ffmpeg.Ffmpeg, status string) error {
	q := fmt.Sprintf("UPDATE `tasks` SET status='%s' WHERE id=%d",
		status, ff.ID)
	res, err := db.db.Query(q)
	if err != nil {
		return err
	}
	res.Close()
	return nil
}

func (db *Db) FinishTask(ff *ffmpeg.Ffmpeg, status string) error {
	q := fmt.Sprintf("UPDATE `tasks` SET status='%s' WHERE id=%d",
		status, ff.ID)
	log.Println(q)
	res, err := db.db.Query(q)
	if err != nil {
		return err
	}
	res.Close()
	db.UnLock(ff.ID)
	return nil
}

func NewDb(serviceID int, sqlURL string) (d *Db, err error) {
	d = &Db{
		serviceID: serviceID,
	}
	sqlURL = strings.Replace(sqlURL, "mysql://", "", 1)
	if !strings.Contains(sqlURL, "@tcp(") {
		reg := regexp.MustCompile(`@(.*)\/`)
		sqlURL = reg.ReplaceAllString(sqlURL, "@tcp($1)/")
	}

	sqlURL = sqlURL + "?parseTime=true"

	// db, err := sql.Open("mysql", "root:<yourMySQLdatabasepassword>@tcp(127.0.0.1:3306)/test")
	log.Println("mysql URL:", sqlURL)
	if d.db, err = sql.Open("mysql", sqlURL); err != nil {
		panic(err.Error())
	}

	d.db.SetConnMaxLifetime(time.Minute * 3)
	d.db.SetMaxOpenConns(10)
	d.db.SetMaxIdleConns(10)

	// check the connection
	if _, err := d.db.Query("show tables"); err != nil {
		panic(err.Error())
	}

	return d, nil
}
