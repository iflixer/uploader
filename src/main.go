package main

import (
	"github.com/joho/godotenv"
	"log"
	"os"
	"strings"
	"uploader/httpserv"
	"uploader/storage"
)

func main() {
	log.Println("START")
	var err error

	if err := godotenv.Load("../.env"); err != nil {
		log.Println("Cant load .env: ", err)
	}

	tmpDir := os.Getenv("TMP_FOLDER")

	s3secret := os.Getenv("S3_SECRET")
	if os.Getenv("S3_SECRET_FILE") != "" {
		s3secret_, err := os.ReadFile(os.Getenv("S3_SECRET_FILE"))
		if err != nil {
			log.Fatal(err)
		}
		s3secret = strings.TrimSpace(string(s3secret_))
	}

	storageService, err := storage.NewService(os.Getenv("S3_BUCKET"), os.Getenv("S3_ENDPOINT"), os.Getenv("S3_KEY_ID"), s3secret)
	if err != nil {
		log.Fatal(err)
	}

	srvServer, err := httpserv.NewServer("3333", tmpDir, storageService)
	if err != nil {
		log.Fatal(err)
	}

	srvServer.Run()

}
