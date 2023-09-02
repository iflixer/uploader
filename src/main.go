package main

import (
	"log"
	"math/rand"
	"os"
	"time"
	"uploader/db"
	"uploader/httpserv"
	"uploader/queue"
	"uploader/s3serv"
)

func main() {
	log.Println("START")

	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1)
	serviceID := r1.Intn(9999999999)

	var err error
	srvDb, err := db.NewDb(serviceID, os.Getenv("MYSQL_URL"))
	if err != nil {
		log.Fatal(err)
	}

	srv3serv, err := s3serv.NewS3serv(os.Getenv("S3_BUCKET"), os.Getenv("S3_ENDPOINT"), os.Getenv("S3_KEY_ID"), os.Getenv("S3_SECRET"))
	if err != nil {
		log.Fatal(err)
	}

	srvQueue, err := queue.NewQueue(srvDb, srv3serv)
	if err != nil {
		log.Fatal(err)
	}

	srvQueue.RunAsync()

	srvServer, err := httpserv.NewServer("3333", srvQueue)
	if err != nil {
		log.Fatal(err)
	}

	srvServer.Run()

}
