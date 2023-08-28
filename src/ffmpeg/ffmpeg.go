package ffmpeg

import (
	"fmt"
	"log"
	"os"
	"strings"
	"uploader/shell"
)

type Ffmpeg struct {
	ID             int
	Name           string
	Magnet         string
	FileName       string
	FileNameResult string
	FileNameLog    string
	FileNameErr    string
}

func (f *Ffmpeg) Convert() error {
	log.Println("Ffmpeg.convert ", f.FileName)
	out, errout, err := shell.Shellout(f.cmdFfmpeg())
	if err != nil {
		log.Printf("convert error: %s\n", err)
		return err
	}
	f.writeLogs(out, errout)
	return nil
}

func (f *Ffmpeg) Probe(full bool) (string, error) {
	log.Println("Ffmpeg.probe ", f.FileName)
	stdout, stderr, err := shell.Shellout(f.cmdProbe(full))
	if err != nil {
		res := fmt.Sprintf("probe error: %s\n %s\n %s", stdout, stderr, err)
		log.Println(res)
		return res, err
	}
	return fmt.Sprintf("%s\n%s", stdout, stderr), nil
}

func (f *Ffmpeg) writeLogs(out, errout string) error {
	fmt.Println("--- stdout ---")
	fmt.Println(out)
	if err := os.WriteFile(f.FileNameLog, []byte(out), 0644); err != nil {
		log.Printf("error saving log: %v\n", err)
		return err
	}
	fmt.Println("--- stderr ---")
	fmt.Println(errout)
	if err := os.WriteFile(f.FileNameErr, []byte(errout), 0644); err != nil {
		log.Printf("error saving errlog: %v\n", err)
		return err
	}
	return nil
}

func (f *Ffmpeg) cmdFfmpeg() string {
	p := `ffmpeg \
	-y \
	-i %s \
	-preset medium \
	-movflags faststart \
	-c:v libx264 \
	-b:v 2M \
	-b:a 200 \
	-pass 1 \
	-vf scale=320:280 \
	-c:a copy \
	-f mp4 \
	%s`
	return fmt.Sprintf(p, f.FileName, f.FileNameResult)
}

func (f *Ffmpeg) cmdProbe(full bool) string {
	if full {
		return f.cmdProbeFull()
	}
	p := `ffprobe \
	-v error \
	%s`
	return fmt.Sprintf(p, f.FileName)
}

func (f *Ffmpeg) cmdProbeFull() string {
	p := `ffprobe \
	%s`
	return fmt.Sprintf(p, f.FileName)
}

func NewFfmpeg(fileName, name string, id int) *Ffmpeg {
	res := strings.Replace(fileName, ".", "_out.", 1)
	l := fileName + "_log"
	er := fileName + "_err"
	f := Ffmpeg{
		ID:             id,
		Name:           name,
		FileName:       fileName,
		FileNameResult: res,
		FileNameLog:    l,
		FileNameErr:    er,
	}
	return &f
}
