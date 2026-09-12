package identity

import (
	"errors"
	"strings"
	"testing"
)

func TestPasswordHashAndPolicy(t *testing.T) {
	password := "Sufficiently.Strong-2026"
	if err := ValidatePassword(password, "operator", "operator@example.test"); err != nil {
		t.Fatal(err)
	}
	encoded, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := VerifyPassword(encoded, password)
	if err != nil || !valid {
		t.Fatalf("valid password rejected: valid=%v err=%v", valid, err)
	}
	valid, err = VerifyPassword(encoded, "wrong-password")
	if err != nil || valid {
		t.Fatalf("wrong password accepted: valid=%v err=%v", valid, err)
	}
}

func TestPasswordRejectsIdentityAndWeakValues(t *testing.T) {
	for _, test := range []struct {
		password string
		rule     PasswordRule
	}{
		{"short", PasswordRuleMinLength},
		{"abcdefghijkl", PasswordRuleDigitRequired},
		{"987654321098", PasswordRuleLetterRequired},
		{"password1234", PasswordRuleCommon},
		{"operator-secure-2026", PasswordRuleIdentity},
		{"mailbox-secure-2026", PasswordRuleIdentity},
	} {
		err := ValidatePassword(test.password, "operator", "mailbox@example.test")
		if !errors.Is(err, ErrWeakPassword) {
			t.Fatalf("weak password accepted: %q", test.password)
		}
		if rule, ok := PasswordRuleFromError(err); !ok || rule != test.rule {
			t.Fatalf("password rule = %q, want %q", rule, test.rule)
		}
	}
}

func TestPasswordPolicyCountsUnicodeCharacters(t *testing.T) {
	password := "安全密码甲乙丙丁戊己庚辛壬0"
	if err := ValidatePassword(password, "operator", "operator@example.test"); err != nil {
		t.Fatalf("unicode password rejected: %v", err)
	}
	tooLong := strings.Repeat("界", PasswordMaxLength+1) + "A1"
	if rule, _ := PasswordRuleFromError(ValidatePassword(tooLong, "operator", "operator@example.test")); rule != PasswordRuleMaxLength {
		t.Fatalf("password rule = %q, want %q", rule, PasswordRuleMaxLength)
	}
	if rule, _ := PasswordRuleFromError(ValidatePassword("abcdefghijkⅧ", "operator", "operator@example.test")); rule != PasswordRuleDigitRequired {
		t.Fatalf("non-decimal number rule = %q, want %q", rule, PasswordRuleDigitRequired)
	}
	if err := ValidatePassword("abcdefghijk١", "operator", "operator@example.test"); err != nil {
		t.Fatalf("unicode decimal digit rejected: %v", err)
	}
}

func TestPasswordChangeRejectsReuse(t *testing.T) {
	password := "Sufficiently.Strong-2026"
	err := ValidatePasswordChange(password, password, "operator", "operator@example.test")
	if rule, ok := PasswordRuleFromError(err); !ok || rule != PasswordRuleReused {
		t.Fatalf("password rule = %q, want %q", rule, PasswordRuleReused)
	}
}

func TestGenerateTemporaryPasswordAlwaysSatisfiesPolicy(t *testing.T) {
	for index := 0; index < 256; index++ {
		password, err := GenerateTemporaryPassword(24, "temporaryadmin", "temporaryadmin@example.test")
		if err != nil {
			t.Fatal(err)
		}
		if len(password) != 24 {
			t.Fatalf("temporary password length = %d, want 24", len(password))
		}
		if err := ValidatePassword(password, "temporaryadmin", "temporaryadmin@example.test"); err != nil {
			t.Fatalf("generated password rejected: %v", err)
		}
	}
}

func TestGenerateTemporaryPasswordRejectsInvalidLength(t *testing.T) {
	if _, err := GenerateTemporaryPassword(PasswordMinLength-1, "operator", "operator@example.test"); err == nil {
		t.Fatal("expected invalid length error")
	}
}
