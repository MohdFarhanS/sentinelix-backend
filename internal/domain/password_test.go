package domain_test

import (
	"testing"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

func TestValidatePassword_Valid(t *testing.T) {
	errs := domain.ValidatePassword("Secret123!")
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidatePassword_TooShort(t *testing.T) {
	errs := domain.ValidatePassword("Sec1!")
	assertContains(t, errs, domain.ErrPasswordTooShort)
}

func TestValidatePassword_NoUppercase(t *testing.T) {
	errs := domain.ValidatePassword("secret123!")
	assertContains(t, errs, domain.ErrPasswordNeedsUppercase)
}

func TestValidatePassword_NoLowercase(t *testing.T) {
	errs := domain.ValidatePassword("SECRET123!")
	assertContains(t, errs, domain.ErrPasswordNeedsLowercase)
}

func TestValidatePassword_NoDigit(t *testing.T) {
	errs := domain.ValidatePassword("SecretAbc!")
	assertContains(t, errs, domain.ErrPasswordNeedsDigit)
}

func TestValidatePassword_NoSpecialChar(t *testing.T) {
	errs := domain.ValidatePassword("Secret1234")
	assertContains(t, errs, domain.ErrPasswordNeedsSpecialChar)
}

func TestValidatePassword_MultipleFailures(t *testing.T) {
	errs := domain.ValidatePassword("abc")
	if len(errs) < 3 {
		t.Errorf("expected multiple errors untuk password lemah, got %d: %v", len(errs), errs)
	}
}

func assertContains(t *testing.T, errs []error, target error) {
	t.Helper()
	for _, e := range errs {
		if e == target {
			return
		}
	}
	t.Errorf("expected errors to contain %v, got %v", target, errs)
}
