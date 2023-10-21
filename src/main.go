package main

import (
	"fmt"
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
	var err error

	s1 := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(s1)
	serviceID := r1.Intn(9999999999)

	apiToken := os.Getenv("API_TOKEN")
	if os.Getenv("API_TOKEN_FILE") != "" {
		apiToken_, err := os.ReadFile(os.Getenv("API_TOKEN_FILE"))
		if err != nil {
			log.Fatal(err)
		}
		apiToken = string(apiToken_)
	}

	mysqlURL := os.Getenv("MYSQL_URL")
	if os.Getenv("MYSQL_URL_FILE") != "" {
		fmt.Println("get MYSQL_URL_FILE")
		mysqlURL_, err := os.ReadFile(os.Getenv("MYSQL_URL_FILE"))
		if err != nil {
			log.Fatal(err)
		}
		mysqlURL = string(mysqlURL_)
	}

	s3secret := os.Getenv("S3_SECRET")
	if os.Getenv("S3_SECRET_FILE") != "" {
		s3secret_, err := os.ReadFile(os.Getenv("S3_SECRET_FILE"))
		if err != nil {
			log.Fatal(err)
		}
		mysqlURL = string(s3secret_)
	}

	srvDb, err := db.NewDb(serviceID, mysqlURL)
	if err != nil {
		log.Fatal(err)
	}

	srv3serv, err := s3serv.NewS3serv(os.Getenv("S3_BUCKET"), os.Getenv("S3_ENDPOINT"), os.Getenv("S3_KEY_ID"), s3secret)
	if err != nil {
		log.Fatal(err)
	}

	srvQueue, err := queue.NewQueue(srvDb, srv3serv)
	if err != nil {
		log.Fatal(err)
	}

	srvQueue.RunAsync()

	srvServer, err := httpserv.NewServer("3333", apiToken, srvQueue, srv3serv)
	if err != nil {
		log.Fatal(err)
	}

	srvServer.Run()

}
