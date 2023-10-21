package ffmpeg

import (
	"fmt"
	"log"
	"os"
	"uploader/shell"
)

type Ffmpeg struct {
	ID             int
	Name           string
	Magnet         string
	OrigFileName   string
	FileName       string
	FileNameResult string
	FileNameLog    string
}

func (f *Ffmpeg) Convert() error {
	if err := f.convert(f.cmdFfmpeg("sd")); err != nil {
		return err
	}
	if err := f.convert(f.cmdFfmpeg("hd")); err != nil {
		return err
	}
	if err := f.convert(f.cmdFfmpeg("")); err != nil {
		return err
	}

	return nil
}

func (f *Ffmpeg) convert(cmd string) error {
	out, errout, err := shell.Shellout(cmd)
	if err != nil {
		log.Printf("convert error: %s, errour: %s\n", err, errout)
		return err
	}
	f.writeLogs(out, errout)
	return nil
}
func (f *Ffmpeg) Probe(full bool) (string, error) {
	cmd := f.cmdProbe(full)
	log.Println("Ffmpeg.probe ", cmd)
	stdout, stderr, err := shell.Shellout(cmd)
	if err != nil {
		res := fmt.Sprintf("probe error: %s\n %s\n %s", stdout, stderr, err)
		log.Println(res)
		return res, err
	}
	return fmt.Sprintf("%s\n%s", stdout, stderr), nil
}

func (f *Ffmpeg) writeLogs(out, errout string) error {
	fmt.Println("--- stderr ---")
	fmt.Println(errout)
	if err := os.WriteFile("/files/"+f.FileNameLog, []byte(errout), 0644); err != nil {
		log.Printf("error saving errlog: %v\n", err)
		return err
	}
	return nil
}

func (f *Ffmpeg) cmdFfmpeg(quality string) string {
	size := ""
	fileResult := f.FileName
	switch quality {
	case "":
		size = "1280:720"
	case "sd":
		size = "512:288"
		fileResult += "_sd"
	case "hd":
		size = "1920:1080"
		fileResult += "_hd"
	}

	p := `ffmpeg \
	-y \
	-hide_banner \
	-fflags +discardcorrupt
	-i %s \
	-preset medium \
	-movflags faststart \
	-c:v libx264 \
	-vf scale=%s \
	-c:a aac \
	-f mp4 \
	%s.mp4`
	return fmt.Sprintf(p, "/files/"+f.FileName, size, "/files/"+fileResult)
}

func (f *Ffmpeg) cmdProbe(full bool) string {
	if full {
		return f.cmdProbeFull()
	}
	p := `ffprobe \
	-hide_banner \
	-v error \
	%s`
	return fmt.Sprintf(p, "/files/"+f.FileName)
}

func (f *Ffmpeg) cmdProbeFull() string {
	p := `ffprobe \
	-hide_banner \
	%s`
	return fmt.Sprintf(p, "/files/"+f.FileName)
}

func NewFfmpeg(origFileName, fileName, name string, id int) *Ffmpeg {
	// res := strings.Replace(fileName, ".", "_out.", 1)
	res := fileName + ".mp4"
	l := fileName + ".txt"
	f := Ffmpeg{
		ID:             id,
		Name:           name,
		OrigFileName:   origFileName,
		FileName:       fileName,
		FileNameResult: res,
		FileNameLog:    l,
	}
	return &f
}
