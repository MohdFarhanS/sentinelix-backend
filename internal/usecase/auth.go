package usecase

import (
	"context"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/pkg/jwt"
	"golang.org/x/crypto/bcrypt"
)

type AuthUsecase struct {
	userRepo	domain.UserRepository
	jwtManager 	*jwt.Manager
}

func NewAuthUsecase(userRepo domain.UserRepository, jwtManager *jwt.Manager) *AuthUsecase {
	return &AuthUsecase{userRepo: userRepo, jwtManager: jwtManager}
}

type RegisterInput struct {
	Email		string
	Password	string
}

type RegisterOutput struct {
	ID		string
	Email	string
}

func (uc *AuthUsecase) Register(ctx context.Context, in RegisterInput) (*RegisterOutput, error) {
	if pwErrs := domain.ValidatePassword(in.Password); len(pwErrs) > 0 {
		return nil, &domain.PasswordValidationError{Errors: pwErrs}
	}

	existing, err := uc.userRepo.FindByEmail(ctx, in.Email)
	if err != nil && err != domain.ErrUserNotFound {
		return nil, err
	}
	if existing != nil {
		return nil, domain.ErrEmailAlreadyExist
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		Email:			in.Email,
		PasswordHash:	string(hash),
	}
	if err := uc.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return &RegisterOutput{ID: user.ID, Email: user.Email}, nil
}

type LoginInput struct {
	Email		string
	Password	string
}

type LoginOutput struct {
	AccessToken string
	ExpiresIn	int64
}

func (uc *AuthUsecase) Login(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	user, err := uc.userRepo.FindByEmail(ctx, in.Email)
	if err != nil {
		if err == domain.ErrUserNotFound {
			return nil, domain.ErrInvalidCredential
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		return nil, domain.ErrInvalidCredential
	}

	token, expiresIn, err := uc.jwtManager.Generate(user.ID)
	if err != nil {
		return nil, err
	}

	return &LoginOutput{AccessToken: token, ExpiresIn: expiresIn}, nil
}