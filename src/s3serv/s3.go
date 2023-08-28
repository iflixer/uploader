package s3serv

// this service should watch the new tasks in DB and process them

type S3serv struct {
}

func (s *S3serv) Add() error {

	return nil
}

func NewS3serv() (*S3serv, error) {
	return &S3serv{}, nil
}
