package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"log"
	"os"
	"strings"
	"uploader/httpserv"
	"uploader/storage"
	"uploader/telegram"
)

func main() {
	log.Println("START")
	var err error

	if err := godotenv.Load("../.env"); err != nil {
		log.Println("Cant load .env: ", err)
	}

	tmpDir := os.Getenv("TMP_FOLDER")
	_ = os.MkdirAll(tmpDir, 0777)
	vManagerUrl := os.Getenv("VMANAGER_URL")

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

	telegramApiToken := os.Getenv("TELEGRAM_APITOKEN")
	if os.Getenv("TELEGRAM_APITOKEN_FILE") != "" {
		telegramApiToken_, err := os.ReadFile(os.Getenv("TELEGRAM_APITOKEN_FILE"))
		if err != nil {
			log.Fatal(err)
		}
		telegramApiToken = strings.TrimSpace(string(telegramApiToken_))
	}

	telegramService, err := telegram.NewService(telegramApiToken)
	if err != nil {
		log.Fatal(err)
	}

	telegramService.Send(telegram.ChanVideo, fmt.Sprintf("uloader started"))

	srvServer, err := httpserv.NewServer("3333", tmpDir, vManagerUrl, storageService, telegramService)
	if err != nil {
		log.Fatal(err)
	}

	srvServer.Run()

}
