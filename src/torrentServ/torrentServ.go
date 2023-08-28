package torrentServ

import (
	"github.com/anacrolix/torrent"
	"log"
)

func Download(url string) (string, bool) {
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = "/files"
	// cfg.NoUpload = true
	// cfg.AcceptPeerConnections = false
	c, _ := torrent.NewClient(cfg)
	defer c.Close()
	t, _ := c.AddMagnet(url) // ubuntu
	<-t.GotInfo()
	info := t.Info()
	log.Printf("Name: %s\n", info.Name)
	log.Printf("Length: %d\n", info.Length)
	/*	log.Println("Files:")
		for _, file := range info.Files {
			log.Printf("Length: %s\n", file.Length)
			for _, path := range file.Path {
				log.Printf("Path: %s\n", path)
			}
			for _, path := range file.PathUtf8 {
				log.Printf("PathUtf8: %s\n", path)
			}
		}*/
	log.Println("start download ", info.Name)
	t.DownloadAll()
	ok := c.WaitAll()
	log.Print("ermahgerd, torrent downloaded")
	return info.Name, ok
}
