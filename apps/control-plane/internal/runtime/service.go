package runtime

import "errors"

type Repository interface{}

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("runtime repository is required")
	}
	return &Service{repository: repository}, nil
}
