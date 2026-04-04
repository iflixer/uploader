package storage

import (
	"fmt"
)

func (s *Service) test() (err error) {
	fmt.Println("testing storage connection:")
	fmt.Println("accessKeyId:", s.accessKeyId)
	fmt.Println("bucketName:", s.bucketName)
	fmt.Println("accessKeySecret(first 3 chars):", s.accessKeySecret[:3])

	fmt.Println("upload test.txt as inbox/test.txt...")
	err = s.Upload("test.txt", "inbox/test.txt")
	// err = s.Upload("/downloads/10690_o.jardim.de.isabel.2024.1080p.web-dl.dual.2.0.mkv", "inbox/tmp.test")
	if err == nil {
		fmt.Println("OK")
	} else {
		fmt.Println("error", err)
	}

	// os.Exit(0) // exit after upload test

	fmt.Println("download inbox/test.mp4...")
	written, err := s.Download("inbox/test.txt", "test.txt")
	if err == nil {
		fmt.Println("OK, bytes:", written)
	} else {
		fmt.Println("error", err)
	}

	// fmt.Println("list inbox/test.txt...")
	// l, err := s.List("inbox/test.txt")
	// if err == nil {
	// 	fmt.Printf(" %+v \n", l)
	// 	fmt.Println("OK")
	// } else {
	// 	fmt.Println("error", err)
	// }
	fmt.Println("testing storage connection done")
	return
}
