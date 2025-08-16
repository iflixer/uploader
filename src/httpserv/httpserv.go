package httpserv

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"uploader/storage"
	"uploader/telegram"

	"github.com/rs/cors"
)

// this service should watch the new tasks in DB and process them

type Server struct {
	mux             *http.ServeMux
	port            string
	storage         *storage.Service
	telegramService *telegram.Service
	tmpDir          string
	vManagerUrl     string
	vManagerAddUrl  string
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
	mux.HandleFunc("/upload", s.upload)
	mux.HandleFunc("/alive", s.alive)
	mux.HandleFunc("/refinalize", s.refinalize)

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

func (s *Server) getRoot(_ http.ResponseWriter, _ *http.Request) {
	// log.Println("get " + r.RequestURI)
}

func (s *Server) alive(_ http.ResponseWriter, _ *http.Request) {
	// log.Println("get " + r.RequestURI)
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
	// log.Println(string(wr))
}

func (s *Server) fileHash(file multipart.File) (string, error) {
	h := md5.New()
	if _, err := io.Copy(h, file); err != nil {
		fmt.Printf("error get hash of file:%s", err)
		return "", err
	}
	_, _ = file.Seek(0, 0)
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (s *Server) fileHashByName(fileName string) (string, error) {
	file, err := os.OpenFile(fileName, os.O_RDONLY, 0644)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	h := md5.New()
	if _, err := io.Copy(h, file); err != nil {
		fmt.Printf("error get hash of file:%s", err)
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (s *Server) stringHash(str string) string {
	h := md5.New()
	h.Write([]byte(str))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func NewServer(port string, tmpDir, vManagerUrl string, storage *storage.Service, telegramService *telegram.Service) (*Server, error) {
	s := &Server{
		port:            port,
		storage:         storage,
		tmpDir:          tmpDir,
		telegramService: telegramService,
		vManagerUrl:     vManagerUrl,
		vManagerAddUrl:  vManagerUrl + "task_add",
	}
	s.init()
	return s, nil
}
