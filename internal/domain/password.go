package domain

import (
	"errors"
	"strings"
	"unicode"
)

const PasswordMinLength = 8

var (
	ErrPasswordTooShort         = errors.New("password must be at least 8 characters")
	ErrPasswordNeedsUppercase   = errors.New("password must contain at least 1 uppercase letter")
	ErrPasswordNeedsLowercase   = errors.New("password must contain at least 1 lowercase letter")
	ErrPasswordNeedsDigit       = errors.New("password must contain at least 1 digit")
	ErrPasswordNeedsSpecialChar = errors.New("password must contain at least 1 special character (e.g. !@#$%^&*)")
)

const specialChars = "!@#$%^&*()_+-=[]{}|;:'\",.<>?/~`\\"

// ValidatePassword ngecek complexity klasik: minimal 8 karakter + kombinasi
// huruf besar, huruf kecil, angka, karakter spesial. Return SEMUA rule yang
// gagal sekaligus (bukan cuma yang pertama ketemu), biar pesan error ke user
// informatif dalam satu kali submit, bukan trial-error satu-satu.
func ValidatePassword(password string) []error {
	var errs []error

	if len(password) < PasswordMinLength {
		errs = append(errs, ErrPasswordTooShort)
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case strings.ContainsRune(specialChars, r):
			hasSpecial = true
		}
	}

	if !hasUpper {
		errs = append(errs, ErrPasswordNeedsUppercase)
	}
	if !hasLower {
		errs = append(errs, ErrPasswordNeedsLowercase)
	}
	if !hasDigit {
		errs = append(errs, ErrPasswordNeedsDigit)
	}
	if !hasSpecial {
		errs = append(errs, ErrPasswordNeedsSpecialChar)
	}

	return errs
}

// PasswordValidationError bungkus semua rule yang gagal jadi satu error,
// supaya bisa diteruskan lewat return value biasa (errors.As-compatible)
// tanpa ubah signature Register() jadi ([]error, error).
type PasswordValidationError struct {
	Errors []error
}

func (e *PasswordValidationError) Error() string {
	msgs := make([]string, len(e.Errors))
	for i, err := range e.Errors {
		msgs[i] = err.Error()
	}
	return strings.Join(msgs, "; ")
}