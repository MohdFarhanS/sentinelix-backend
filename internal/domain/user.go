package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrUserNotFound 		= errors.New("user not found")
	ErrEmailAlreadyExist 	= errors.New("email already resitered")
	ErrInvalidCredential 	= errors.New("invalid email or password")
)

type User struct {
	ID				string
	Email			string
	PasswordHash	string
	CreatedAt		time.Time
}

// UserRepository didefinikan di domain, diimplementasikan di repository/postgres.
// Usecase cuman bergantung ke interface ini, tidak tahu soal pgx/SQL sama sekali.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	FindByEmail(ctx context.Context, email string) (*User, error)
}