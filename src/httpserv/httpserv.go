package httpserv

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"github.com/rs/cors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"uploader/queue"
	"uploader/s3serv"
)

// this service should watch the new tasks in DB and process them

type Server struct {
	mux   *http.ServeMux
	port  string
	queue *queue.Queue
	s3    *s3serv.S3serv
}

type Resp struct {
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	Errorcode string `json:"errorcode,omitempty"`
	Text      string `json:"text,omitempty"`
}

func (s *Server) init() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.getRoot)
	mux.HandleFunc("/favicon.ico", s.get404)
	mux.HandleFunc("/convert", s.convert)
	mux.HandleFunc("/probe", s.probe)
	mux.HandleFunc("/check_storage", s.checkStorage)
	mux.HandleFunc("/list_storage", s.listStorage)
	mux.HandleFunc("/check", s.check)
	mux.HandleFunc("/tasks", s.tasks)
	mux.HandleFunc("/files/", s.files)

	s.mux = mux
}

func (s *Server) Run() {
	log.Println("Start http server on :" + s.port)
	handler := cors.Default().Handler(s.mux)
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedHeaders:   []string{"Content-Disposition", "Content-Range", "Content-Type"},
		AllowCredentials: true,
		Debug:            false,
	})
	handler = c.Handler(handler)
	if err := http.ListenAndServe(":"+s.port, handler); err != nil {
		log.Fatal(err)
	}
}

func (s *Server) getRoot(w http.ResponseWriter, r *http.Request) {
	// log.Println("get " + r.RequestURI)
}

func (s *Server) files(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL.Path + " range: " + r.Header.Get("Range"))
	http.ServeFile(w, r, r.URL.Path)
}

func (s *Server) get404(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}

func (s *Server) returnResp(w http.ResponseWriter, txt string, err error) {
	re := &Resp{
		Success:   true,
		Error:     "",
		Errorcode: "",
		Text:      txt,
	}
	code := http.StatusOK
	if err != nil {
		re.Success = false
		re.Error = err.Error()
	}
	wr, _ := json.Marshal(re)
	http.Error(w, string(wr), code)
	log.Println(string(wr))
}

func (s *Server) fileHash(file multipart.File) (string, error) {
	h := md5.New()
	if _, err := io.Copy(h, file); err != nil {
		fmt.Errorf("error get hash of file:%s", err)
		return "", err
	}
	file.Seek(0, 0)
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (s *Server) fileHashByName(fileName string) (string, error) {
	file, err := os.OpenFile(fileName, os.O_RDONLY, 0644)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := md5.New()
	if _, err := io.Copy(h, file); err != nil {
		fmt.Errorf("error get hash of file:%s", err)
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (s *Server) stringHash(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func NewServer(port string, queue *queue.Queue, srv3serv *s3serv.S3serv) (*Server, error) {
	s := &Server{
		port:  port,
		queue: queue,
		s3:    srv3serv,
	}
	s.init()
	return s, nil
}
